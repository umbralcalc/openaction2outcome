package score

import (
	"fmt"
	"sort"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// Report is the full evaluation of a submission against a set of marks.
type Report struct {
	ModelName     string      `json:"model_name"`
	SchemaVersion string      `json:"schema_version"`
	MarkScores    []MarkScore `json:"mark_scores"`
	Summary       Summary     `json:"summary"`
	// Skipped lists marks present in the corpus that the submission did not
	// predict, and predictions that referenced no known mark.
	Skipped Skipped `json:"skipped"`
}

// Summary aggregates the per-mark scores across both scores.
type Summary struct {
	NumMarksScored int `json:"num_marks_scored"`

	// Decision.
	NumSignKnown int     `json:"num_sign_known"`
	SignAccuracy float64 `json:"sign_accuracy"` // over sign-known marks
	MeanRegret   float64 `json:"mean_regret"`
	TotalRegret  float64 `json:"total_regret"`

	// Calibration.
	OverlapRate         float64            `json:"overlap_rate"`
	MeanOverlapFraction float64            `json:"mean_overlap_fraction"`
	MeanCramerDistance  float64            `json:"mean_cramer_distance"`
	NumConfidentlyWrong int                `json:"num_confidently_wrong"`
	Calibration         []CalibrationPoint `json:"calibration"`
}

// CalibrationPoint is one point of the calibration curve: of all scored marks,
// the fraction whose PIT (model CDF at the mark's central truth) falls at or
// below Nominal. For a well-calibrated model Empirical ≈ Nominal.
type CalibrationPoint struct {
	Nominal   float64 `json:"nominal"`
	Empirical float64 `json:"empirical"`
}

// Skipped records coverage gaps.
type Skipped struct {
	UnpredictedMarks   []string `json:"unpredicted_marks,omitempty"`
	UnmatchedPredicted []string `json:"unmatched_predictions,omitempty"`
}

// ScoreSubmission evaluates a submission against the supplied marks. Marks are
// matched to predictions by ID; coverage gaps in either direction are reported
// rather than silently dropped.
func ScoreSubmission(marks []schema.Mark, sub schema.Submission, opt Options) Report {
	markByID := make(map[string]schema.Mark, len(marks))
	for _, m := range marks {
		markByID[m.ID] = m
	}
	predByID := make(map[string]schema.Prediction, len(sub.Predictions))
	for _, p := range sub.Predictions {
		predByID[p.MarkID] = p
	}

	r := Report{ModelName: sub.ModelName, SchemaVersion: sub.SchemaVersion}

	// Deterministic ordering: iterate marks in sorted ID order.
	ids := make([]string, 0, len(markByID))
	for id := range markByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		p, ok := predByID[id]
		if !ok {
			r.Skipped.UnpredictedMarks = append(r.Skipped.UnpredictedMarks, id)
			continue
		}
		r.MarkScores = append(r.MarkScores, ScoreMark(markByID[id], p, opt))
	}
	for _, p := range sub.Predictions {
		if _, ok := markByID[p.MarkID]; !ok {
			r.Skipped.UnmatchedPredicted = append(r.Skipped.UnmatchedPredicted, p.MarkID)
		}
	}

	r.Summary = summarise(r.MarkScores)
	return r
}

func summarise(scores []MarkScore) Summary {
	s := Summary{NumMarksScored: len(scores)}
	if len(scores) == 0 {
		return s
	}
	var signCorrect, overlaps int
	var overlapFracSum, cramerSum float64
	pits := make([]float64, 0, len(scores))

	for _, ms := range scores {
		if ms.Decision.MarkSignKnown {
			s.NumSignKnown++
			if ms.Decision.SignCorrect {
				signCorrect++
			}
		}
		s.TotalRegret += ms.Decision.Regret
		if ms.Calibration.IntervalsOverlap {
			overlaps++
		}
		overlapFracSum += ms.Calibration.OverlapFraction
		cramerSum += ms.Calibration.CramerDistance
		if ms.Calibration.ConfidentlyWrong {
			s.NumConfidentlyWrong++
		}
		pits = append(pits, ms.Calibration.PIT)
	}

	n := float64(len(scores))
	if s.NumSignKnown > 0 {
		s.SignAccuracy = float64(signCorrect) / float64(s.NumSignKnown)
	}
	s.MeanRegret = s.TotalRegret / n
	s.OverlapRate = float64(overlaps) / n
	s.MeanOverlapFraction = overlapFracSum / n
	s.MeanCramerDistance = cramerSum / n
	s.Calibration = calibrationCurve(pits)
	return s
}

// calibrationCurve turns the per-mark PITs into a curve at deciles. Empirical[k]
// is the fraction of PITs <= the nominal level — uniform PITs give Empirical ≈
// Nominal. (A per-mark-width-aware calibration — integrating the PIT over each
// mark's own distribution rather than at its central point — is a planned
// refinement.)
func calibrationCurve(pits []float64) []CalibrationPoint {
	levels := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9}
	out := make([]CalibrationPoint, len(levels))
	n := float64(len(pits))
	for i, lvl := range levels {
		c := 0
		for _, pit := range pits {
			if pit <= lvl {
				c++
			}
		}
		out[i] = CalibrationPoint{Nominal: lvl, Empirical: float64(c) / n}
	}
	return out
}

// String renders a compact human-readable summary for CLI output.
func (r Report) String() string {
	s := r.Summary
	out := fmt.Sprintf("model=%q marks_scored=%d\n", r.ModelName, s.NumMarksScored)
	out += fmt.Sprintf("  Decision: sign_accuracy=%.3f (over %d sign-known) total_regret=%.4g mean_regret=%.4g\n",
		s.SignAccuracy, s.NumSignKnown, s.TotalRegret, s.MeanRegret)
	out += fmt.Sprintf("  Calibration: overlap_rate=%.3f mean_overlap_frac=%.3f mean_cramer=%.4g confidently_wrong=%d\n",
		s.OverlapRate, s.MeanOverlapFraction, s.MeanCramerDistance, s.NumConfidentlyWrong)
	if len(r.Skipped.UnpredictedMarks) > 0 {
		out += fmt.Sprintf("  skipped (unpredicted marks): %v\n", r.Skipped.UnpredictedMarks)
	}
	if len(r.Skipped.UnmatchedPredicted) > 0 {
		out += fmt.Sprintf("  skipped (unmatched predictions): %v\n", r.Skipped.UnmatchedPredicted)
	}
	return out
}
