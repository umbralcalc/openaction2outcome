// Package its is the controlled interrupted-time-series estimator: the time-domain
// analogue of the regression-discontinuity estimators in internal/rdd and the
// difference-in-differences estimator in internal/did. It identifies the population
// effect of a sharp intervention over a post-intervention window by fitting a
// segmented regression to the TREATED-minus-CONTROL difference series, so common
// short-window confounders (meteorology, regional/secondary pollution, season) that
// move both series net out before the break is read.
//
// Why this design, and why now. The ULEZ-style emission-zone seam is a sharp policy
// instant with no running-variable cutoff, so neither RDD nor a clean 2x2 DiD around
// a single window fits it: the estimand is a level/slope break in a monthly series,
// accumulated over the post window. A controlled ITS is the canonical design for that
// — and a family of such anchors at different policy stringencies (the 2019/2021/2023
// ULEZ expansions, plus European LEZs) is exactly what a future bridge mark spans.
//
// The identifying assumption is PARALLEL TRENDS IN TIME: absent the action, the
// treated-minus-control difference would have continued its pre-intervention path.
// So the estimator ships the diagnostics that defend it — a pre-period parallelism
// slope (the difference should be flat pre-break), placebo intervention dates in the
// pre-period (which should show no break), and a no-anticipation check just before
// the instant — reported, never folded into the effect.
//
// Inference is Newey-West (Bartlett-kernel HAC), because monthly air-quality
// residuals are serially correlated and an OLS standard error would understate the
// sampling variance. The honest interval then folds the SPECIFICATION spread
// (pre-window length x seasonal harmonics x level-only-vs-level+slope x meteorology
// adjustment) on top of that sampling variance, exactly as the RDD marks fold the
// bandwidth x order x kernel spread — so a plug-in that fixed one specification would
// report a narrower, less honest interval.
package its

import (
	"math"
	"sort"
)

// Point is one month's aggregated treated and control levels on the running-time
// axis. T is months since a fixed epoch (any consistent integer-spaced scale);
// Treated and Control are the cross-station mean outcomes that month. Covar carries
// optional per-month covariates (e.g. meteorology) used by meteorology-adjusted
// specifications. Months must be unique and are sorted by the estimator.
type Point struct {
	T       float64
	Month   string
	Treated float64
	Control float64
	Covar   map[string]float64
}

// Spec is one controlled-ITS specification. The model is fit on the difference
// series D(t) = Treated(t) - Control(t):
//
//	D(t) = b0 + b1*(t-t0) + b2*post + b3*post*(t-t0) + seasonal(t) [+ meteo]
//
// where post = 1{t >= t0}. b2 is the level break in the difference at the instant
// and b3 its slope change; the post-window average effect is b2 + b3*mean_post(t-t0).
type Spec struct {
	Name      string
	PreStartT float64  // earliest month (inclusive) entering the fit
	Harmonics int      // seasonal harmonic pairs at period 12 (0, 1 or 2)
	Slope     bool     // include the post-intervention slope change b3
	Meteo     []string // covariate names to adjust for (empty = unadjusted)
	NWLag     int      // Newey-West Bartlett lag (months)
	Weight    float64  // model-average weight (defaults to equal if zero)
}

// FitResult is one specification's fit: the post-window average effect, its HAC
// sampling variance, the underlying level/slope break, and goodness/diagnostic
// summaries.
type FitResult struct {
	Effect      float64 // post-window average of b2 + b3*(t-t0)
	Var         float64 // HAC sampling variance of Effect
	LevelBreak  float64 // b2
	SlopeBreak  float64 // b3 (0 when Spec.Slope is false)
	NPre        int
	NPost       int
	N           int
	K           int     // number of regressors
	Resid1Corr  float64 // lag-1 residual autocorrelation (autocorrelation diagnostic)
	OK          bool
}

// SE returns the HAC standard error of the effect.
func (f FitResult) SE() float64 { return math.Sqrt(f.Var) }

// designRow builds the regressor row for one observation under a spec.
func (s Spec) designRow(p Point, t0 float64, post bool) []float64 {
	dt := p.T - t0
	row := []float64{1, dt}
	if post {
		row = append(row, 1)
	} else {
		row = append(row, 0)
	}
	if s.Slope {
		if post {
			row = append(row, dt)
		} else {
			row = append(row, 0)
		}
	}
	for k := 1; k <= s.Harmonics; k++ {
		w := 2 * math.Pi * float64(k) * p.T / 12
		row = append(row, math.Sin(w), math.Cos(w))
	}
	for _, name := range s.Meteo {
		row = append(row, p.Covar[name])
	}
	return row
}

// postContrast returns the linear-combination vector c such that c·beta is the
// post-window average effect: it loads 1 on the post (level-break) column and
// mean_post(t-t0) on the post-slope column (if present), 0 elsewhere.
func (s Spec) postContrast(k int, meanPostDt float64) []float64 {
	c := make([]float64, k)
	c[2] = 1 // post level-break column
	if s.Slope {
		c[3] = meanPostDt
	}
	return c
}

// Fit fits one specification on [PreStartT, end] and returns the post-window
// average effect with its HAC variance. t0 is the running time of the FIRST post
// month (the intervention instant rounded to the post bucket); months in
// [PreStartT, t0) are pre, months >= t0 are post. Transition months excluded by the
// caller simply do not appear in pts.
func Fit(pts []Point, t0 float64, spec Spec) FitResult {
	if spec.NWLag <= 0 {
		spec.NWLag = 3
	}
	// Select and order the months in window.
	var rows []Point
	for _, p := range pts {
		if p.T >= spec.PreStartT {
			rows = append(rows, p)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].T < rows[j].T })

	var X [][]float64
	var y []float64
	var nPre, nPost int
	var sumPostDt float64
	for _, p := range rows {
		post := p.T >= t0
		X = append(X, spec.designRow(p, t0, post))
		y = append(y, p.Treated-p.Control)
		if post {
			nPost++
			sumPostDt += p.T - t0
		} else {
			nPre++
		}
	}
	res := FitResult{NPre: nPre, NPost: nPost, N: len(rows)}
	// Require an adequate sample on BOTH sides of the break. The post-side minimum
	// also guards placebo/no-anticipation fits placed near the window edge: with only
	// a couple of post-break months a level term fits almost perfectly and reports an
	// over-confident (tiny-SE) spurious break, which would corrupt the validity gate.
	if len(X) == 0 || nPost < 4 || nPre < 4 {
		return res
	}
	k := len(X[0])
	res.K = k
	if len(rows) <= k+1 {
		return res
	}
	beta, xtxInv, ok := ols(X, y)
	if !ok {
		return res
	}
	res.LevelBreak = beta[2]
	if spec.Slope {
		res.SlopeBreak = beta[3]
	}
	// Residuals and lag-1 autocorrelation.
	resid := make([]float64, len(y))
	for i := range y {
		resid[i] = y[i] - dot(X[i], beta)
	}
	res.Resid1Corr = lag1corr(resid)

	// Newey-West HAC covariance of beta, then variance of the contrast.
	V := neweyWest(X, resid, xtxInv, spec.NWLag)
	meanPostDt := sumPostDt / float64(nPost)
	c := spec.postContrast(k, meanPostDt)
	res.Effect = dot(c, beta)
	res.Var = quadForm(c, V)
	res.OK = res.Var >= 0 && !math.IsNaN(res.Effect) && !math.IsInf(res.Effect, 0)
	return res
}

// --- model-averaged honest interval -----------------------------------------

// SpecEstimate pairs a spec name with its fitted effect/variance/weight.
type SpecEstimate struct {
	Name   string
	Effect float64
	Var    float64
	Weight float64
}

// BMAResult is the model-averaged effect over a specification set, mirroring
// sbi.BMAResult so an ITS mark maps to schema.Distribution exactly as an RDD mark
// does. The mixture is an equal- (or supplied-) weight mixture of per-spec normals
// N(Effect, Var); its total variance decomposes into the within-spec sampling
// variance and the between-spec identification variance.
type BMAResult struct {
	Central    float64
	TotalSD    float64
	WithinVar  float64 // E_s[Var] — sampling
	BetweenVar float64 // Var_s[Effect] — identification/specification
	Specs      []SpecEstimate
}

// EstimateBMA fits every spec and model-averages. Specs that fail to fit (too few
// usable months, singular design) are dropped; weights renormalise over survivors.
func EstimateBMA(pts []Point, t0 float64, specs []Spec) BMAResult {
	var ests []SpecEstimate
	var wsum float64
	for _, s := range specs {
		f := Fit(pts, t0, s)
		if !f.OK {
			continue
		}
		w := s.Weight
		if w <= 0 {
			w = 1
		}
		ests = append(ests, SpecEstimate{Name: s.Name, Effect: f.Effect, Var: f.Var, Weight: w})
		wsum += w
	}
	var r BMAResult
	if len(ests) == 0 || wsum == 0 {
		return r
	}
	for i := range ests {
		ests[i].Weight /= wsum
	}
	for _, e := range ests {
		r.Central += e.Weight * e.Effect
		r.WithinVar += e.Weight * e.Var
	}
	for _, e := range ests {
		d := e.Effect - r.Central
		r.BetweenVar += e.Weight * d * d
	}
	r.TotalSD = math.Sqrt(r.WithinVar + r.BetweenVar)
	sort.SliceStable(ests, func(i, j int) bool { return ests[i].Weight > ests[j].Weight })
	r.Specs = ests
	return r
}

// cdf of the mixture at x.
func (r BMAResult) cdf(x float64) float64 {
	var c float64
	for _, s := range r.Specs {
		sd := math.Sqrt(s.Var)
		if sd <= 0 {
			if x >= s.Effect {
				c += s.Weight
			}
			continue
		}
		c += s.Weight * normalCDF((x-s.Effect)/sd)
	}
	return c
}

// Quantile inverts the mixture CDF by bisection.
func (r BMAResult) Quantile(p float64) float64 {
	if len(r.Specs) == 0 {
		return 0
	}
	if p <= 0 {
		p = 1e-6
	}
	if p >= 1 {
		p = 1 - 1e-6
	}
	lo, hi := r.Central-50*r.TotalSD-1, r.Central+50*r.TotalSD+1
	for i := 0; i < 200; i++ {
		mid := 0.5 * (lo + hi)
		if r.cdf(mid) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return 0.5 * (lo + hi)
}

// Interval returns the central interval at the given level (e.g. 0.95).
func (r BMAResult) Interval(level float64) (lo, hi float64) {
	a := (1 - level) / 2
	return r.Quantile(a), r.Quantile(1 - a)
}

// Samples returns k deterministic stratified inverse-CDF samples of the mixture.
func (r BMAResult) Samples(k int) []float64 {
	out := make([]float64, k)
	for i := 0; i < k; i++ {
		out[i] = r.Quantile((float64(i) + 0.5) / float64(k))
	}
	return out
}

func normalCDF(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }
