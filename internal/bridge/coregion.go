package bridge

import (
	"fmt"
	"sort"
)

// Coregionalisation (multi-output) kernels let a bridge borrow strength across
// correlated outputs — related outcomes, or the same outcome in related
// jurisdictions — so anchors on one output inform the discrepancy on another. This
// is the Intrinsic Coregionalisation Model (ICM): the joint covariance of the
// discrepancy at (x, output a) and (x', output b) factorises as
//
//	K((x,a),(x',b)) = B[a][b] · k_base(x, x')
//
// where k_base is a single-output kernel over the policy variable and B is a
// positive-semidefinite "coregionalisation matrix" encoding how strongly the
// outputs co-vary. B is built as B = W Wᵀ + diag(κ): the rank-r factor W couples
// the outputs and the κ floor keeps each output's own variance, guaranteeing B ⪰ 0
// and hence a valid joint Gram. The base kernel's load-bearing trust-decay
// assumption is unchanged; B adds the cross-output trust-transfer assumption, and
// both ship as provenance.
//
// The single-output bridge pipeline (gp.go) consumes a scalar Kernel, so
// Coregionalization also implements Kernel for a chosen primary output — its
// within-output covariance — letting it drop into the existing conditioner
// unchanged. The genuinely joint conditioning over (x, output) pairs is built on
// CovMulti / JointGram, the reusable multi-output machinery, which the next bridge
// (a multi-jurisdiction family) will condition on directly.

// Coregionalization is an ICM kernel over a base single-output kernel and a PSD
// coregionalisation matrix B over the outputs.
type Coregionalization struct {
	Base        Kernel      // single-output covariance over the policy variable
	B           [][]float64 // PSD coregionalisation matrix (NumOutputs × NumOutputs)
	PrimaryOut  int         // the output the scalar Kernel methods report on
	familyLabel string
}

// NewCoregionalization builds B = W Wᵀ + diag(κ) from a rank-r factor W
// (NumOutputs × r) and a per-output variance floor κ, guaranteeing B ⪰ 0. primary
// is the output the scalar Kernel interface reports (its own within-output kernel).
func NewCoregionalization(base Kernel, W [][]float64, kappa []float64, primary int) (Coregionalization, error) {
	p := len(kappa)
	if p == 0 || len(W) != p {
		return Coregionalization{}, fmt.Errorf("coregion: W must be NumOutputs×r and kappa length NumOutputs (got W=%d×_, kappa=%d)", len(W), p)
	}
	if primary < 0 || primary >= p {
		return Coregionalization{}, fmt.Errorf("coregion: primary output %d out of range [0,%d)", primary, p)
	}
	B := make([][]float64, p)
	for a := 0; a < p; a++ {
		B[a] = make([]float64, p)
		for b := 0; b < p; b++ {
			var s float64
			for r := range W[a] {
				s += W[a][r] * W[b][r]
			}
			B[a][b] = s
		}
		B[a][a] += kappa[a]
	}
	return Coregionalization{Base: base, B: B, PrimaryOut: primary, familyLabel: "coregionalised-" + base.Name()}, nil
}

// NumOutputs is the number of coupled outputs.
func (k Coregionalization) NumOutputs() int { return len(k.B) }

// CovMulti is the joint covariance K((x1,a),(x2,b)) = B[a][b]·k_base(x1,x2).
func (k Coregionalization) CovMulti(x1 float64, a int, x2 float64, b int) float64 {
	return k.B[a][b] * k.Base.Cov(x1, x2)
}

// JointGram assembles the full multi-output Gram over the given (x, output) pairs —
// the reusable machinery for jointly conditioning a discrepancy across outputs.
func (k Coregionalization) JointGram(xs []float64, outputs []int) ([][]float64, error) {
	if len(xs) != len(outputs) {
		return nil, fmt.Errorf("coregion: xs and outputs length mismatch (%d vs %d)", len(xs), len(outputs))
	}
	n := len(xs)
	g := make([][]float64, n)
	for i := 0; i < n; i++ {
		g[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			g[i][j] = k.CovMulti(xs[i], outputs[i], xs[j], outputs[j])
		}
	}
	return g, nil
}

// --- single-output Kernel interface (the PrimaryOut within-output covariance) ----

func (k Coregionalization) Name() string { return k.familyLabel }

func (k Coregionalization) Cov(x1, x2 float64) float64 {
	return k.B[k.PrimaryOut][k.PrimaryOut] * k.Base.Cov(x1, x2)
}

func (k Coregionalization) Variance() float64 {
	return k.B[k.PrimaryOut][k.PrimaryOut] * k.Base.Variance()
}

func (k Coregionalization) Params() map[string]float64 {
	out := map[string]float64{}
	for name, v := range k.Base.Params() {
		out["base_"+name] = v
	}
	out["primary_output"] = float64(k.PrimaryOut)
	out["num_outputs"] = float64(k.NumOutputs())
	// Ship the coregionalisation matrix (deterministic key ordering) as provenance.
	p := k.NumOutputs()
	idx := make([]int, 0, p)
	for a := 0; a < p; a++ {
		idx = append(idx, a)
	}
	sort.Ints(idx)
	for _, a := range idx {
		for b := a; b < p; b++ {
			out[fmt.Sprintf("B_%d_%d", a, b)] = k.B[a][b]
		}
	}
	return out
}
