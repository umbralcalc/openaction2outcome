package bridge

import (
	"fmt"
	"math"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// The tractability gate is the Axis-B detector of the determinism doctrine. The
// deterministic calibrator is honest only inside the linear-Gaussian / mildly
// nonlinear regime; past genuine nonlinearity or non-Gaussianity a tidy Gaussian
// interval over a multimodal truth is a miscalibrated lie. This gate tests whether
// a mechanism remains in that regime BEFORE a deterministic interval is minted, and
// ships its verdict as provenance. A mechanism that fails is not given a
// deterministic interval — it is flagged and routed to the deferred sampling path.
//
// What it measures (Axis B = tractability, NOT Axis A = causal structure — a
// directed causal model can be perfectly linear-Gaussian). Over the fitted θ
// posterior it compares two propagations of the prediction map g(θ):
//
//   - the unscented (second-order) propagated variance, and
//   - the linearised / EKF (first-order, ∇gᵀΣ∇g) variance.
//
// Their relative gap measures how much curvature g carries across the posterior
// bulk: zero for a linear-Gaussian mechanism, growing with nonlinearity. It also
// measures the skew of the propagated sigma values.
//
// The deeper failure mode — and the one the spec actually quarantines for sampling
// — is a *non-Gaussian θ posterior* (a multimodal or otherwise intractable
// likelihood). The discrepancy GP absorbs a great deal of smooth mechanism
// curvature in the prediction map, so a curved m alone is usually still tractable;
// what breaks determinism is when the Laplace (Gaussian) approximation to the θ
// posterior is itself wrong. The gate measures this directly via the LAPLACE MISFIT:
// at sigma points it compares the true negative-log-posterior to the fit's quadratic
// approximation. For a linear-Gaussian model the NLP is exactly quadratic (misfit 0
// → pass); a genuinely non-Gaussian posterior bends away from the quadratic (misfit
// large → fail). All three statistics must sit under tolerance for the gate to pass.

// Rung labels for the inference record (the spec's three-rung ladder).
const (
	rungClosedForm          = "closed-form"
	rungDeterministicMoment = "deterministic-moment"
	rungSampled             = "sampled"
)

// GateVerdict is the tractability-gate result, shipped as provenance so a consumer
// sees that determinism was earned, not assumed.
type GateVerdict struct {
	// Pass is true iff the mechanism is certified in the deterministic regime.
	Pass bool
	// Rung is the inference rung the gate recommends: closed-form (linear),
	// deterministic-moment (mildly nonlinear, passed), or sampled (failed → defer).
	Rung string
	// Linear records whether the mechanism was detected linear-in-θ (exact regime).
	Linear bool
	// NonlinearityGap is the relative gap between the unscented and linearised
	// propagated variances of g(θ): |σ²_UT − σ²_EKF| / σ²_UT. Curvature proxy.
	NonlinearityGap float64
	// Skew is |standardised third moment| of the propagated sigma values: a
	// non-Gaussianity proxy.
	Skew float64
	// LaplaceMisfit is the largest gap (in nats) between the true negative-log-
	// posterior and its Laplace quadratic approximation across the sigma points —
	// the direct test of whether the θ posterior is Gaussian. ~0 for linear-Gaussian.
	LaplaceMisfit float64
	// Tolerances used (shipped for auditability).
	NonlinearityTol float64
	SkewTol         float64
	MisfitTol       float64
	// Reason is a human-readable verdict line for the dossier.
	Reason string
}

// gate tolerances. They are deliberately modest: the gate's job is to catch
// genuine nonlinearity/non-Gaussianity, not to police round-off. A mechanism whose
// unscented and linearised predictive variances agree to within 5%, with negligible
// propagated skew, is in the deterministic regime.
const (
	defaultNonlinearityTol = 0.05
	defaultSkewTol         = 0.30
	// A Laplace misfit above ~0.5 nats at the ~1σ sigma points means the Gaussian
	// posterior approximation mis-states tail mass by more than ~e^0.5 ≈ 1.6×, which
	// is enough to make the deterministic interval dishonest.
	defaultMisfitTol = 0.5
)

// TractabilityGate fits θ deterministically and tests whether the prediction map
// g(θ) = m(query;θ) + δ̄(θ) stays in the deterministic regime over the θ posterior.
// It is the entry condition for CalibrateMoment and the routing decision for the
// (deferred) sampling path. useMarginal selects the same θ weighting the calibrator
// will use, so the gate sees exactly the posterior the interval will rest on.
func TractabilityGate(mech Mechanism, anchors []Anchor, query float64, k Kernel, useMarginal bool) (GateVerdict, error) {
	if err := CheckBracketing(anchors, query); err != nil {
		return GateVerdict{}, err
	}
	xs := make([]float64, len(anchors))
	mu := make([]float64, len(anchors))
	noise := make([]float64, len(anchors))
	for i, a := range anchors {
		v, ok := a.noiseVar()
		if !ok {
			return GateVerdict{}, fmt.Errorf("bridge: anchor %q has no usable posterior width", a.MarkID)
		}
		xs[i], mu[i], noise[i] = a.X, a.mean(), v
	}
	gp, err := newGPConditioner(xs, noise, k, defaultJitter)
	if err != nil {
		return GateVerdict{}, err
	}
	fit, err := fitMechanismMoment(mech, xs, mu, noise, gp, useMarginal)
	if err != nil {
		return GateVerdict{}, err
	}

	w := gp.condMeanWeights(query)
	g := func(theta []float64) float64 {
		resid := make([]float64, len(xs))
		for i := range xs {
			resid[i] = mu[i] - mech.Predict(xs[i], theta)
		}
		return mech.Predict(query, theta) + dot(w, resid)
	}

	// Unscented (second-order) propagated variance.
	_, utVar, err := unscentedPropagate(g, fit.mean, fit.cov)
	if err != nil {
		return GateVerdict{}, err
	}
	// Linearised / EKF (first-order) propagated variance ∇gᵀ Σ ∇g.
	grad := scalarGrad(g, fit.mean)
	ekfVar := quad(fit.cov, grad)

	gap := 0.0
	if utVar > 0 {
		gap = math.Abs(utVar-ekfVar) / utVar
	} else if ekfVar > 0 {
		gap = 1
	}
	skew := propagatedSkew(g, fit.mean, fit.cov)
	misfit := laplaceMisfit(fit)

	v := GateVerdict{
		Linear:          fit.linear,
		NonlinearityGap: gap,
		Skew:            skew,
		LaplaceMisfit:   misfit,
		NonlinearityTol: defaultNonlinearityTol,
		SkewTol:         defaultSkewTol,
		MisfitTol:       defaultMisfitTol,
	}
	v.Pass = gap <= defaultNonlinearityTol && skew <= defaultSkewTol && misfit <= defaultMisfitTol
	switch {
	case v.Pass && fit.linear:
		v.Rung = rungClosedForm
		v.Reason = "linear-Gaussian: the prediction map is linear in θ and the θ posterior is exactly Gaussian, so the deterministic interval is exact."
	case v.Pass:
		v.Rung = rungDeterministicMoment
		v.Reason = fmt.Sprintf("mildly nonlinear and in-regime: variance gap %.1f%% ≤ %.0f%%, skew %.2f ≤ %.2f, Laplace misfit %.2f ≤ %.2f nats; the moment-propagated interval is trustworthy.",
			100*gap, 100*defaultNonlinearityTol, skew, defaultSkewTol, misfit, defaultMisfitTol)
	default:
		v.Rung = rungSampled
		v.Reason = fmt.Sprintf("OUT OF REGIME: variance gap %.1f%% (tol %.0f%%), skew %.2f (tol %.2f), or Laplace misfit %.2f (tol %.2f nats) exceeded — the θ posterior is too non-Gaussian for a deterministic interval; route to the deferred sampling path.",
			100*gap, 100*defaultNonlinearityTol, skew, defaultSkewTol, misfit, defaultMisfitTol)
	}
	return v, nil
}

// laplaceMisfit measures how non-Gaussian the θ posterior is: the largest gap (in
// nats) between the true negative-log-posterior increment NLP(θ)−NLP(θ̂) and its
// Laplace quadratic ½(θ−θ̂)ᵀH(θ−θ̂) at points placed along the posterior's principal
// axes at 1σ and 2σ. With cov = L Lᵀ, a step of s·L·eⱼ has quadratic value ½s²
// exactly, so a Gaussian posterior gives misfit ~0 and a bent (multimodal /
// skewed / heavy-tailed) posterior gives a large misfit.
func laplaceMisfit(fit momentFit) float64 {
	d := len(fit.mean)
	L, ok := cholesky(fit.cov)
	if !ok {
		L, ok = cholesky(addDiag(fit.cov, 1e-12))
		if !ok {
			return 0
		}
	}
	base := fit.negLogPosterior(fit.mean)
	var maxMisfit float64
	for _, s := range []float64{1.0, 2.0} {
		quad := 0.5 * s * s // ½(s·eⱼ)ᵀeⱼ along a single principal axis
		for j := 0; j < d; j++ {
			for _, sign := range []float64{+1, -1} {
				th := append([]float64(nil), fit.mean...)
				for r := 0; r < d; r++ {
					th[r] += sign * s * L[r][j]
				}
				actual := fit.negLogPosterior(th) - base
				if m := math.Abs(actual - quad); m > maxMisfit {
					maxMisfit = m
				}
			}
		}
	}
	return maxMisfit
}

// SchemaRecord converts the gate verdict into the schema's wire-format inference
// record, stamping the rung it recommends. This is the provenance a bridge mark
// ships so a consumer sees that determinism was earned, not assumed.
func (v GateVerdict) SchemaRecord() schema.InferenceRecord {
	return schema.InferenceRecord{
		Rung: v.Rung,
		Tractability: &schema.TractabilityVerdict{
			Pass:            v.Pass,
			Linear:          v.Linear,
			NonlinearityGap: v.NonlinearityGap,
			Skew:            v.Skew,
			LaplaceMisfit:   v.LaplaceMisfit,
			NonlinearityTol: v.NonlinearityTol,
			SkewTol:         v.SkewTol,
			MisfitTol:       v.MisfitTol,
			Reason:          v.Reason,
		},
	}
}

// scalarGrad is the central-difference gradient of a scalar map g at θ.
func scalarGrad(g func([]float64) float64, theta []float64) []float64 {
	d := len(theta)
	out := make([]float64, d)
	for j := 0; j < d; j++ {
		h := 1e-6 * (1 + math.Abs(theta[j]))
		tp := append([]float64(nil), theta...)
		tm := append([]float64(nil), theta...)
		tp[j] += h
		tm[j] -= h
		out[j] = (g(tp) - g(tm)) / (2 * h)
	}
	return out
}

// propagatedSkew is |standardised third moment| of the unscented sigma values of g,
// a deterministic non-Gaussianity proxy. A symmetric (e.g. linear) push-through has
// skew ~0; a strongly nonlinear map skews the propagated distribution.
func propagatedSkew(g func([]float64) float64, mean []float64, cov [][]float64) float64 {
	d := len(mean)
	const alpha, kappa = 1.0, 0.0 // wider spread than the calibrator's UT, to probe curvature
	lambda := alpha*alpha*(float64(d)+kappa) - float64(d)
	scale := float64(d) + lambda
	L, ok := cholesky(scaleMat(cov, scale))
	if !ok {
		L, ok = cholesky(scaleMat(addDiag(cov, 1e-12), scale))
		if !ok {
			return 0
		}
	}
	wm := make([]float64, 2*d+1)
	wm[0] = lambda / scale
	for i := 1; i < 2*d+1; i++ {
		wm[i] = 1.0 / (2 * scale)
	}
	vals := make([]float64, 2*d+1)
	vals[0] = g(mean)
	for j := 0; j < d; j++ {
		cp := append([]float64(nil), mean...)
		cm := append([]float64(nil), mean...)
		for r := 0; r < d; r++ {
			cp[r] += L[r][j]
			cm[r] -= L[r][j]
		}
		vals[1+j] = g(cp)
		vals[1+d+j] = g(cm)
	}
	var m1, m2, m3 float64
	for i := range vals {
		m1 += wm[i] * vals[i]
	}
	for i := range vals {
		dv := vals[i] - m1
		m2 += wm[i] * dv * dv
		m3 += wm[i] * dv * dv * dv
	}
	if m2 <= 0 {
		return 0
	}
	return math.Abs(m3) / math.Pow(m2, 1.5)
}
