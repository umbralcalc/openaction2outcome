package bridge

import "math"

// Kernel is a Gaussian-process covariance family over the policy variable. It is
// the load-bearing trust-decay assumption of a bridge mark — it encodes how fast
// simulator-trust decays with distance from an anchor — and ships openly as
// provenance. The pinning (variance → anchor noise at a pin, bulging between) is
// produced by GP conditioning in gp.go, NOT baked into the kernel, so any
// stationary kernel automatically yields the bridge geometry.
type Kernel interface {
	// Name identifies the family for provenance and dossier rendering.
	Name() string
	// Cov is k(x1, x2), the prior covariance between the discrepancy at two points.
	Cov(x1, x2 float64) float64
	// Variance is k(x, x) — the prior (and maximum mid-bridge) discrepancy variance.
	Variance() float64
	// Params returns the hyperparameters for provenance.
	Params() map[string]float64
}

// SquaredExponential is the smoothest standard kernel: k = σf² exp(-d²/(2ℓ²)).
// Its trust decays sharply and smoothly with distance from an anchor.
type SquaredExponential struct {
	SigmaF      float64 // prior discrepancy sd (σf); Variance = σf²
	Lengthscale float64 // ℓ: distance over which discrepancy decorrelates
}

func (k SquaredExponential) Name() string { return "squared-exponential" }

func (k SquaredExponential) Cov(x1, x2 float64) float64 {
	d := x1 - x2
	return k.SigmaF * k.SigmaF * math.Exp(-(d*d)/(2*k.Lengthscale*k.Lengthscale))
}

func (k SquaredExponential) Variance() float64 { return k.SigmaF * k.SigmaF }

func (k SquaredExponential) Params() map[string]float64 {
	return map[string]float64{"sigma_f": k.SigmaF, "lengthscale": k.Lengthscale}
}

// Matern52 is rougher than the squared-exponential (twice mean-square
// differentiable), so trust decays off-anchor with heavier tails. It is the
// natural sensitivity contrast: if τ(query) moves a lot between this and the
// squared-exponential, the estimate is kernel-driven and is flagged.
//
//	k = σf² (1 + √5 d/ℓ + 5 d²/(3ℓ²)) exp(-√5 d/ℓ)
type Matern52 struct {
	SigmaF      float64
	Lengthscale float64
}

func (k Matern52) Name() string { return "matern52" }

func (k Matern52) Cov(x1, x2 float64) float64 {
	d := math.Abs(x1-x2) / k.Lengthscale
	s := math.Sqrt(5)
	return k.SigmaF * k.SigmaF * (1 + s*d + (5.0/3.0)*d*d) * math.Exp(-s*d)
}

func (k Matern52) Variance() float64 { return k.SigmaF * k.SigmaF }

func (k Matern52) Params() map[string]float64 {
	return map[string]float64{"sigma_f": k.SigmaF, "lengthscale": k.Lengthscale}
}
