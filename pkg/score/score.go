package score

import (
	"math"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// TrackA captures decision-value consistency for one mark (BRIEF §4, Track A).
type TrackA struct {
	// MarkSignKnown is true when the mark's honest interval excludes zero, so
	// the sign of the true effect is itself identified.
	MarkSignKnown bool `json:"mark_sign_known"`
	// SignCorrect reports whether the model's central estimate has the same sign
	// as the mark's central estimate. Meaningful only when MarkSignKnown.
	SignCorrect bool `json:"sign_correct"`
	// Regret is the decision regret: zero unless the mark's sign is known and the
	// model points the other way, in which case it is the magnitude of the
	// mark's central effect (the value foregone by the wrong decision). No
	// penalty accrues where the mark is itself unsure of the sign.
	Regret float64 `json:"regret"`
}

// TrackB captures calibration-against-truth for one mark (BRIEF §4, Track B).
type TrackB struct {
	// IntervalsOverlap is true when the model's predicted interval overlaps the
	// mark's honest interval at all.
	IntervalsOverlap bool `json:"intervals_overlap"`
	// OverlapFraction is the overlap width as a fraction of the mark's interval
	// width (0 = disjoint, 1 = mark interval fully covered by the model's).
	OverlapFraction float64 `json:"overlap_fraction"`
	// CramerDistance is the integrated squared difference between the model's and
	// mark's CDFs — a proper, distribution-vs-distribution CRPS generalisation.
	// Lower is better; 0 means identical distributions.
	CramerDistance float64 `json:"cramer_distance"`
	// PIT is the model's predictive CDF evaluated at the mark's central estimate.
	// Aggregated across marks it drives the calibration curve; a well-calibrated
	// model yields PITs uniform on [0,1].
	PIT float64 `json:"pit"`
	// ConfidentlyWrong flags the fair version of a hallucinated counterfactual:
	// the model is narrow AND wrong while the mark is itself narrow AND known.
	ConfidentlyWrong bool `json:"confidently_wrong"`
}

// MarkScore is the full per-mark result.
type MarkScore struct {
	MarkID string `json:"mark_id"`
	TrackA TrackA `json:"track_a"`
	TrackB TrackB `json:"track_b"`
}

// Options tunes the confidently-wrong detector's "narrow" thresholds. Widths
// are compared in the effect's native units; zero values fall back to defaults.
type Options struct {
	// MarkNarrowWidth: a mark with interval width <= this is "narrow-and-known".
	MarkNarrowWidth float64
	// ModelNarrowWidth: a model prediction with interval width <= this is "narrow".
	ModelNarrowWidth float64
}

func (o Options) markNarrow() float64 {
	if o.MarkNarrowWidth > 0 {
		return o.MarkNarrowWidth
	}
	return math.Inf(1) // by default no mark counts as narrow until calibrated per-seam
}

func (o Options) modelNarrow() float64 {
	if o.ModelNarrowWidth > 0 {
		return o.ModelNarrowWidth
	}
	return math.Inf(1)
}

// ScoreMark evaluates one prediction against one mark on both tracks.
func ScoreMark(m schema.Mark, p schema.Prediction, opt Options) MarkScore {
	return MarkScore{
		MarkID: m.ID,
		TrackA: scoreTrackA(m, p),
		TrackB: scoreTrackB(m, p, opt),
	}
}

func scoreTrackA(m schema.Mark, p schema.Prediction) TrackA {
	mc := m.Effect.Central
	iv := m.Effect.Interval
	signKnown := iv != nil && (iv.Lower > 0 || iv.Upper < 0)

	a := TrackA{MarkSignKnown: signKnown}
	if !signKnown {
		a.SignCorrect = true // unidentified sign: no decision to get wrong
		return a
	}
	a.SignCorrect = sameSign(mc, p.Effect.Central)
	if !a.SignCorrect {
		a.Regret = math.Abs(mc)
	}
	return a
}

func scoreTrackB(m schema.Mark, p schema.Prediction, opt Options) TrackB {
	var b TrackB
	mi := m.Effect.Interval
	pi := p.Effect.Interval

	if mi != nil && pi != nil {
		lo := math.Max(mi.Lower, pi.Lower)
		hi := math.Min(mi.Upper, pi.Upper)
		if hi >= lo {
			b.IntervalsOverlap = true
			if w := mi.Upper - mi.Lower; w > 0 {
				b.OverlapFraction = (hi - lo) / w
			} else if mi.Lower >= pi.Lower && mi.Upper <= pi.Upper {
				b.OverlapFraction = 1 // degenerate (point) mark inside model interval
			}
		}
	}

	b.CramerDistance = cramerDistance(p.Effect, m.Effect)
	if pit, ok := cdfEval(p.Effect, m.Effect.Central); ok {
		b.PIT = pit
	}

	// Confidently-wrong: narrow-and-wrong model while the mark is narrow-and-known.
	if mi != nil && pi != nil {
		markW := mi.Upper - mi.Lower
		modelW := pi.Upper - pi.Lower
		markNarrowKnown := markW <= opt.markNarrow() && (mi.Lower > 0 || mi.Upper < 0)
		modelNarrow := modelW <= opt.modelNarrow()
		modelWrong := m.Effect.Central < pi.Lower || m.Effect.Central > pi.Upper
		b.ConfidentlyWrong = markNarrowKnown && modelNarrow && modelWrong
	}
	return b
}

// cramerDistance approximates ∫ (F_pred(x) - F_mark(x))^2 dx by trapezoidal
// integration over a grid spanning both distributions' support. This is the
// distribution-vs-distribution generalisation of CRPS (the Cramér / energy
// distance): when the mark degenerates to a point mass it reduces to the
// ordinary CRPS of the prediction at that point.
func cramerDistance(pred, mark schema.Distribution) float64 {
	grid := integrationGrid(pred, mark)
	if len(grid) < 2 {
		return 0
	}
	var total float64
	prevX := grid[0]
	prevD := cdfDiffSq(pred, mark, prevX)
	for i := 1; i < len(grid); i++ {
		x := grid[i]
		d := cdfDiffSq(pred, mark, x)
		total += 0.5 * (prevD + d) * (x - prevX)
		prevX, prevD = x, d
	}
	return total
}

func cdfDiffSq(pred, mark schema.Distribution, x float64) float64 {
	fp, okp := cdfEval(pred, x)
	fm, okm := cdfEval(mark, x)
	if !okp || !okm {
		return 0
	}
	diff := fp - fm
	return diff * diff
}

// integrationGrid builds a sorted, padded, deduplicated grid covering both
// distributions' support, dense enough for a stable trapezoidal estimate.
func integrationGrid(pred, mark schema.Distribution) []float64 {
	pts := append(support(pred), support(mark)...)
	if len(pts) == 0 {
		return nil
	}
	lo, hi := pts[0], pts[0]
	for _, x := range pts {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	if hi == lo {
		return []float64{lo}
	}
	pad := 0.1 * (hi - lo)
	lo, hi = lo-pad, hi+pad
	const n = 512
	grid := make([]float64, n+1)
	step := (hi - lo) / n
	for i := range grid {
		grid[i] = lo + float64(i)*step
	}
	return grid
}

func sameSign(a, b float64) bool {
	switch {
	case a > 0:
		return b > 0
	case a < 0:
		return b < 0
	default:
		return b == 0
	}
}
