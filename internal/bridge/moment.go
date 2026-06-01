package bridge

import (
	"fmt"
	"math"

	"github.com/umbralcalc/stochadex/pkg/inference"
)

// This file is the DETERMINISTIC moment-propagation calibrator — rungs 1 and 2 of
// the inference ladder, and the determinism-first default for the causal layer. It
// computes the posterior over τ(query) with NO Monte Carlo: θ is fit to the
// anchors by deterministic Gauss–Newton (a Gaussian posterior over θ from the
// likelihood Hessian + the prior), and that θ-Gaussian is pushed through the
// prediction map g(θ) = m(query;θ) + δ̄(θ) by the unscented transform (sigma points
// placed deterministically). The GP discrepancy variance is θ-independent and
// added once, exactly as in the closed-form conditioner.
//
// Why this is the default. For a mechanism that is *linear in θ* with Gaussian
// anchor noise the whole pipeline is exact — the θ posterior is exactly Gaussian
// and the unscented push-through reproduces the analytic mean and variance to
// round-off (proven in moment_test.go and the linear-Gaussian-limit study). For a
// mildly nonlinear mechanism the sigma points capture curvature to second order.
// There is no sampling seed, so a moment-propagated bridge re-mints byte-for-byte.
//
// Relationship to the SMC calibrators (calibrate.go). Those fit θ with stochadex
// SMC and condition δ in closed form; they are deterministic given a seed but carry
// finite-particle noise. This calibrator removes the particle cloud entirely in the
// regime where it is justified — which the tractability gate (tractability.go)
// certifies before this is used.

// DeterministicResult bundles a gated deterministic calibration: the tractability
// verdict (always present) and the posterior (present iff the gate passed). When
// the gate fails, Posterior is nil and the caller must route to the deferred
// sampling path rather than mint a miscalibrated deterministic interval.
type DeterministicResult struct {
	Verdict   GateVerdict
	Posterior *BridgePosterior // nil when the gate failed
}

// CalibrateDeterministic is the determinism-first entry point: it runs the
// tractability gate and, only if the mechanism is certified in the deterministic
// regime, mints the moment-propagated interval — stamping it with the gate's rung.
// If the gate fails it returns the verdict with a nil posterior so the caller flags
// the bridge and hands it to the sampling route; it never papers a nonlinear /
// non-Gaussian mechanism over with a tidy Gaussian interval. This is the layer's
// "determinism-first, not determinism-only" guarantee made executable.
func CalibrateDeterministic(mech Mechanism, anchors []Anchor, query float64, k Kernel, useMarginal bool) (DeterministicResult, error) {
	verdict, err := TractabilityGate(mech, anchors, query, k, useMarginal)
	if err != nil {
		return DeterministicResult{}, err
	}
	if !verdict.Pass {
		return DeterministicResult{Verdict: verdict}, nil
	}
	post, err := CalibrateMoment(mech, anchors, query, k, useMarginal)
	if err != nil {
		return DeterministicResult{Verdict: verdict}, err
	}
	post.Rung = verdict.Rung
	return DeterministicResult{Verdict: verdict, Posterior: &post}, nil
}

// CalibrateMoment is the deterministic moment-propagation calibrator. It returns a
// Gaussian posterior over τ(query): central = unscented-propagated mean, the
// between-θ variance from the propagated sigma spread, and the within (GP
// discrepancy) variance from closed-form conditioning. useMarginal selects the θ
// likelihood weighting: false = modular cut (diagonal anchor noise), true = the GP
// marginal-likelihood weighting (K+Σ)⁻¹, matching Calibrate / CalibrateMarginal.
func CalibrateMoment(mech Mechanism, anchors []Anchor, query float64, k Kernel, useMarginal bool) (BridgePosterior, error) {
	var zero BridgePosterior
	if err := CheckBracketing(anchors, query); err != nil {
		return zero, err
	}

	xs := make([]float64, len(anchors))
	mu := make([]float64, len(anchors))
	noise := make([]float64, len(anchors))
	for i, a := range anchors {
		v, ok := a.noiseVar()
		if !ok {
			return zero, fmt.Errorf("bridge: anchor %q has no usable posterior width (need std_dev, interval, or samples)", a.MarkID)
		}
		xs[i], mu[i], noise[i] = a.X, a.mean(), v
	}

	gp, err := newGPConditioner(xs, noise, k, defaultJitter)
	if err != nil {
		return zero, err
	}

	fit, err := fitMechanismMoment(mech, xs, mu, noise, gp, useMarginal)
	if err != nil {
		return zero, err
	}

	// θ-independent GP pieces, computed once.
	condVar := gp.condVar(query)
	w := gp.condMeanWeights(query) // (K+Σ)⁻¹ k⋆

	// g(θ) = m(query;θ) + δ̄(θ), δ̄ = wᵀ(μ − m(·;θ)).
	g := func(theta []float64) float64 {
		resid := make([]float64, len(xs))
		for i := range xs {
			resid[i] = mu[i] - mech.Predict(xs[i], theta)
		}
		return mech.Predict(query, theta) + dot(w, resid)
	}

	central, betweenVar, err := unscentedPropagate(g, fit.mean, fit.cov)
	if err != nil {
		return zero, err
	}

	rung := rungDeterministicMoment
	if fit.linear {
		rung = rungClosedForm
	}
	total := condVar + betweenVar
	mix := gaussMixture{means: []float64{central}, vars: []float64{total}, weights: []float64{1}}
	return BridgePosterior{
		Query:    query,
		Central:  central,
		TotalSD:  math.Sqrt(total),
		GPVar:    condVar,
		ThetaVar: betweenVar,
		Kernel:   k.Name(),
		Method:   "moment",
		Rung:     rung,
		dist:     mix,
	}, nil
}

// momentFit is the deterministic Gaussian posterior over θ: its mean (the MAP) and
// covariance (the inverse Gauss–Newton Hessian at the MAP). linear records whether
// the mechanism was detected to be linear-in-θ over the fit region, in which case
// the fit (and the whole pipeline) is exact.
type momentFit struct {
	mean   []float64
	cov    [][]float64
	linear bool

	// Likelihood pieces retained so the tractability gate can test how Gaussian the
	// θ posterior actually is (the Laplace-misfit / non-Gaussianity check). dataPrec
	// is W (diagonal 1/σ² for the cut, or (K+Σ)⁻¹ for the marginal); priorPrec and
	// m0 are the Gaussian prior precision and mean.
	xs        []float64
	mu        []float64
	dataPrec  [][]float64
	m0        []float64
	priorPrec []float64
	mech      Mechanism
}

// negLogPosterior is the (unnormalised) negative log posterior over θ used in the
// fit, ½ rᵀW r + ½ (θ−m0)ᵀ diag(priorPrec) (θ−m0) with r = μ − m(·;θ). The gate
// compares this against its Laplace quadratic to measure non-Gaussianity.
func (f momentFit) negLogPosterior(theta []float64) float64 {
	resid := make([]float64, len(f.xs))
	for i := range f.xs {
		resid[i] = f.mu[i] - f.mech.Predict(f.xs[i], theta)
	}
	nlp := 0.5 * quad(f.dataPrec, resid)
	for j := range theta {
		dv := theta[j] - f.m0[j]
		nlp += 0.5 * f.priorPrec[j] * dv * dv
	}
	return nlp
}

// fitMechanismMoment fits θ by Gauss–Newton on the anchor residuals with a Gaussian
// prior (derived from the mechanism's declared priors). The data precision is the
// diagonal anchor noise (modular cut) or the GP marginal precision (K+Σ)⁻¹ when
// useMarginal is set — the same two weightings the SMC calibrators use. The
// returned covariance is the Laplace posterior covariance (inverse Hessian at the
// optimum); for a linear mechanism it is the exact Gaussian posterior covariance.
func fitMechanismMoment(mech Mechanism, xs, mu, noise []float64, gp *gpConditioner, useMarginal bool) (momentFit, error) {
	priors, _ := mech.Priors()
	d := mech.ParamDim()
	m0, pvar, ok := priorMoments(priors)
	if !ok || len(m0) != d {
		return momentFit{}, fmt.Errorf("bridge: moment propagation needs Gaussian-summarisable priors for all %d params", d)
	}
	// Prior precision (diagonal). A degenerate prior variance is treated as a very
	// weak prior so the anchors dominate, matching the SMC fits' wide priors.
	priorPrec := make([]float64, d)
	for j := range priorPrec {
		if pvar[j] > 0 {
			priorPrec[j] = 1.0 / pvar[j]
		}
	}

	// Data precision W: diagonal 1/σ² (cut) or the dense (K+Σ)⁻¹ (marginal).
	var dataPrec [][]float64
	if useMarginal {
		dataPrec = gp.ginv
	} else {
		dataPrec = diag(invEach(noise))
	}

	theta := append([]float64(nil), m0...)
	var cov [][]float64
	// Gauss–Newton iterations. For a linear mechanism one step from any start is
	// exact; the loop converges in one step and the extra iterations are no-ops.
	const maxIter = 12
	for iter := 0; iter < maxIter; iter++ {
		resid := make([]float64, len(xs)) // r = μ − m(·;θ)
		for i := range xs {
			resid[i] = mu[i] - mech.Predict(xs[i], theta)
		}
		J := residJacobian(mech, xs, theta) // ∂m(xᵢ;θ)/∂θⱼ  (n×d)

		// Hessian H = JᵀWJ + diag(priorPrec); gradient of −log posterior
		// grad = −JᵀW r + priorPrec·(θ − m0).
		Wr := matVec(dataPrec, resid) // W r  (n)
		grad := make([]float64, d)
		for j := 0; j < d; j++ {
			var s float64
			for i := range xs {
				s -= J[i][j] * Wr[i]
			}
			grad[j] = s + priorPrec[j]*(theta[j]-m0[j])
		}
		WJ := matMat(dataPrec, J) // (n×d)
		H := make([][]float64, d)
		for a := 0; a < d; a++ {
			H[a] = make([]float64, d)
			for b := 0; b < d; b++ {
				var s float64
				for i := range xs {
					s += J[i][a] * WJ[i][b]
				}
				H[a][b] = s
			}
			H[a][a] += priorPrec[a]
		}
		Hinv, okInv := invert(H)
		if !okInv {
			return momentFit{}, fmt.Errorf("bridge: θ posterior Hessian singular (anchors do not identify θ); add anchors or tighten priors")
		}
		step := matVec(Hinv, grad)
		var maxStep float64
		for j := 0; j < d; j++ {
			theta[j] -= step[j]
			if math.Abs(step[j]) > maxStep {
				maxStep = math.Abs(step[j])
			}
		}
		cov = Hinv
		if maxStep < 1e-12 {
			break
		}
	}

	return momentFit{
		mean: theta, cov: cov, linear: mechanismLinear(mech, xs, theta),
		xs: xs, mu: mu, dataPrec: dataPrec, m0: m0, priorPrec: priorPrec, mech: mech,
	}, nil
}

// residJacobian returns the n×d Jacobian Jᵢⱼ = ∂m(xᵢ;θ)/∂θⱼ by central finite
// differences (deterministic). The step scales with the parameter magnitude.
func residJacobian(mech Mechanism, xs, theta []float64) [][]float64 {
	d := len(theta)
	J := make([][]float64, len(xs))
	for i := range xs {
		J[i] = make([]float64, d)
	}
	for j := 0; j < d; j++ {
		h := 1e-6 * (1 + math.Abs(theta[j]))
		tp := append([]float64(nil), theta...)
		tm := append([]float64(nil), theta...)
		tp[j] += h
		tm[j] -= h
		for i := range xs {
			J[i][j] = (mech.Predict(xs[i], tp) - mech.Predict(xs[i], tm)) / (2 * h)
		}
	}
	return J
}

// mechanismLinear tests whether m(x;θ) is linear in θ at the fit point by checking
// that a second-order central difference (the diagonal curvature) vanishes for
// every parameter at every anchor. A linear mechanism makes the moment pipeline
// exact and earns the closed-form rung label.
func mechanismLinear(mech Mechanism, xs, theta []float64) bool {
	for j := range theta {
		h := 1e-3 * (1 + math.Abs(theta[j]))
		tp := append([]float64(nil), theta...)
		tm := append([]float64(nil), theta...)
		tp[j] += h
		tm[j] -= h
		for i := range xs {
			f0 := mech.Predict(xs[i], theta)
			// Second central difference ≈ ∂²m/∂θⱼ². It is ~round-off (≪1) for a
			// linear mechanism and O(1) for genuine curvature, so the relative
			// threshold cleanly separates the two.
			curv := (mech.Predict(xs[i], tp) - 2*f0 + mech.Predict(xs[i], tm)) / (h * h)
			if math.Abs(curv) > 1e-4*(1+math.Abs(f0)) {
				return false
			}
		}
	}
	return true
}

// unscentedPropagate pushes the Gaussian N(mean, cov) over θ through the scalar map
// g by the scaled unscented transform: 2d+1 deterministically-placed sigma points,
// recombined into the propagated mean and variance. For a linear g it reproduces
// the analytic mean g(mean) and variance ∇gᵀ cov ∇g exactly. It carries no RNG.
func unscentedPropagate(g func([]float64) float64, mean []float64, cov [][]float64) (mu, variance float64, err error) {
	d := len(mean)
	// Scaled UT parameters. α=1 spreads the sigma points across the full ±√d·Σ
	// bulk so the transform captures g's curvature to second order (rung 2); κ=0,
	// β=2 is the Gaussian-optimal choice. The UT is exact for a linear g at ANY α,
	// so this remains exact in the linear-Gaussian limit. λ = α²(d+κ) − d.
	const alpha, beta, kappa = 1.0, 2.0, 0.0
	lambda := alpha*alpha*(float64(d)+kappa) - float64(d)
	scale := float64(d) + lambda

	L, ok := cholesky(scaleMat(cov, scale))
	if !ok {
		// cov should be PD (inverse Hessian); fall back to a jittered factor.
		L, ok = cholesky(scaleMat(addDiag(cov, 1e-12), scale))
		if !ok {
			return 0, 0, fmt.Errorf("bridge: θ covariance not positive-definite for sigma-point factorisation")
		}
	}

	wm := make([]float64, 2*d+1)
	wc := make([]float64, 2*d+1)
	wm[0] = lambda / scale
	wc[0] = lambda/scale + (1 - alpha*alpha + beta)
	for i := 1; i < 2*d+1; i++ {
		wm[i] = 1.0 / (2 * scale)
		wc[i] = 1.0 / (2 * scale)
	}

	gvals := make([]float64, 2*d+1)
	gvals[0] = g(mean)
	for j := 0; j < d; j++ {
		colP := make([]float64, d)
		colM := make([]float64, d)
		for r := 0; r < d; r++ {
			colP[r] = mean[r] + L[r][j] // L's column j is sqrt(scale·cov) column
			colM[r] = mean[r] - L[r][j]
		}
		gvals[1+j] = g(colP)
		gvals[1+d+j] = g(colM)
	}

	for i := range gvals {
		mu += wm[i] * gvals[i]
	}
	for i := range gvals {
		dv := gvals[i] - mu
		variance += wc[i] * dv * dv
	}
	if variance < 0 {
		variance = 0
	}
	return mu, variance, nil
}

// priorMoments summarises each declared prior as a Gaussian (mean, variance) for
// the deterministic θ fit. Uniform[lo,hi] → ((lo+hi)/2, (hi−lo)²/12); truncated
// normal → (Mu, Sigma²). Returns ok=false if any prior is not summarisable, which
// routes the caller to the SMC path instead.
func priorMoments(priors []inference.Prior) (mean, variance []float64, ok bool) {
	mean = make([]float64, len(priors))
	variance = make([]float64, len(priors))
	for i, p := range priors {
		switch pr := p.(type) {
		case *inference.UniformPrior:
			mean[i] = 0.5 * (pr.Lo + pr.Hi)
			variance[i] = (pr.Hi - pr.Lo) * (pr.Hi - pr.Lo) / 12.0
		case *inference.TruncatedNormalPrior:
			mean[i] = pr.Mu
			variance[i] = pr.Sigma * pr.Sigma
		default:
			return nil, nil, false
		}
	}
	return mean, variance, true
}

// --- small matrix helpers used only by the moment calibrator -------------------

func diag(v []float64) [][]float64 {
	n := len(v)
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
		m[i][i] = v[i]
	}
	return m
}

func invEach(v []float64) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		if x > 0 {
			out[i] = 1.0 / x
		}
	}
	return out
}

// matMat returns a·b for a (p×q) and b (q×r).
func matMat(a, b [][]float64) [][]float64 {
	p := len(a)
	q := len(b)
	r := 0
	if q > 0 {
		r = len(b[0])
	}
	out := make([][]float64, p)
	for i := 0; i < p; i++ {
		out[i] = make([]float64, r)
		for k := 0; k < q; k++ {
			aik := a[i][k]
			if aik == 0 {
				continue
			}
			for j := 0; j < r; j++ {
				out[i][j] += aik * b[k][j]
			}
		}
	}
	return out
}

func scaleMat(a [][]float64, s float64) [][]float64 {
	out := make([][]float64, len(a))
	for i := range a {
		out[i] = make([]float64, len(a[i]))
		for j := range a[i] {
			out[i][j] = s * a[i][j]
		}
	}
	return out
}

func addDiag(a [][]float64, eps float64) [][]float64 {
	out := make([][]float64, len(a))
	for i := range a {
		out[i] = append([]float64(nil), a[i]...)
		out[i][i] += eps
	}
	return out
}
