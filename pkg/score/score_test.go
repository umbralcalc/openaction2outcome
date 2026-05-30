package score

import (
	"math"
	"testing"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

func ptr(f float64) *float64 { return &f }

// gaussianMark builds a sign-known mark with a Gaussian-ish effect distribution
// centred at mu with a 95% interval of half-width 1.96*sd.
func gaussianMark(id string, mu, sd float64) schema.Mark {
	return schema.Mark{
		SchemaVersion: schema.SchemaVersion,
		ID:            id,
		Series:        schema.SeriesAreaFunding,
		RDDType:       schema.Sharp,
		Design: schema.Design{
			Cutoff:    0,
			Direction: schema.AboveTreated,
		},
		Effect: schema.Distribution{
			Central: mu,
			StdDev:  ptr(sd),
			Interval: &schema.Interval{
				Level: 0.95, Lower: mu - 1.96*sd, Upper: mu + 1.96*sd,
			},
		},
	}
}

func pred(mu, sd float64) schema.Prediction {
	return schema.Prediction{
		Effect: schema.Distribution{
			Central: mu,
			StdDev:  ptr(sd),
			Interval: &schema.Interval{
				Level: 0.95, Lower: mu - 1.96*sd, Upper: mu + 1.96*sd,
			},
		},
	}
}

func TestDecisionSignAndRegret(t *testing.T) {
	m := gaussianMark("m1", 2.0, 0.3) // sign-known positive
	// Model points the right way.
	a := scoreDecision(m, pred(1.5, 0.4))
	if !a.MarkSignKnown || !a.SignCorrect || a.Regret != 0 {
		t.Fatalf("expected sign-known, correct, zero regret; got %+v", a)
	}
	// Model points the wrong way -> regret = |mark central|.
	a = scoreDecision(m, pred(-1.0, 0.4))
	if a.SignCorrect || a.Regret != 2.0 {
		t.Fatalf("expected wrong sign with regret 2.0; got %+v", a)
	}
	// Mark whose interval straddles zero: sign unknown, no decision to get wrong.
	mUnsure := gaussianMark("m2", 0.1, 1.0)
	a = scoreDecision(mUnsure, pred(-5, 0.1))
	if a.MarkSignKnown || !a.SignCorrect || a.Regret != 0 {
		t.Fatalf("expected sign-unknown free pass; got %+v", a)
	}
}

func TestCalibrationOverlapAndCramer(t *testing.T) {
	m := gaussianMark("m1", 2.0, 0.5)
	// Identical prediction: full overlap, ~zero Cramér distance, PIT ~ 0.5.
	b := scoreCalibration(m, pred(2.0, 0.5), Options{})
	if !b.IntervalsOverlap {
		t.Fatal("identical distributions should overlap")
	}
	if b.OverlapFraction < 0.99 {
		t.Fatalf("identical intervals should fully overlap; got %.3f", b.OverlapFraction)
	}
	if b.CramerDistance > 1e-3 {
		t.Fatalf("identical distributions should have ~0 Cramér distance; got %g", b.CramerDistance)
	}
	if math.Abs(b.PIT-0.5) > 0.05 {
		t.Fatalf("PIT at the centre should be ~0.5; got %.3f", b.PIT)
	}

	// Far-away, confident prediction: disjoint intervals, larger Cramér distance.
	far := scoreCalibration(m, pred(10.0, 0.5), Options{})
	if far.IntervalsOverlap {
		t.Fatal("distant intervals should be disjoint")
	}
	if far.CramerDistance <= b.CramerDistance {
		t.Fatalf("distant prediction should have larger Cramér distance: near=%g far=%g",
			b.CramerDistance, far.CramerDistance)
	}
}

func TestConfidentlyWrong(t *testing.T) {
	m := gaussianMark("m1", 2.0, 0.1) // narrow + sign-known
	opt := Options{MarkNarrowWidth: 1.0, ModelNarrowWidth: 1.0}

	// Narrow model, badly wrong centre -> flagged.
	b := scoreCalibration(m, pred(-3.0, 0.1), opt)
	if !b.ConfidentlyWrong {
		t.Fatal("narrow-and-wrong model vs narrow-and-known mark should be confidently wrong")
	}
	// Wide (humble) model that misses the centre -> NOT flagged (it admitted doubt).
	b = scoreCalibration(m, pred(-3.0, 5.0), opt)
	if b.ConfidentlyWrong {
		t.Fatal("a wide, uncertain model must not be flagged confidently wrong")
	}
}

func TestScoreSubmissionCoverage(t *testing.T) {
	marks := []schema.Mark{gaussianMark("a", 1, 0.2), gaussianMark("b", -1, 0.2)}
	sub := schema.Submission{
		SchemaVersion: schema.SchemaVersion,
		ModelName:     "test",
		Predictions: []schema.Prediction{
			{MarkID: "a", Effect: pred(1, 0.2).Effect},
			{MarkID: "ghost", Effect: pred(0, 1).Effect},
		},
	}
	r := ScoreSubmission(marks, sub, Options{})
	if r.Summary.NumMarksScored != 1 {
		t.Fatalf("expected 1 scored mark; got %d", r.Summary.NumMarksScored)
	}
	if len(r.Skipped.UnpredictedMarks) != 1 || r.Skipped.UnpredictedMarks[0] != "b" {
		t.Fatalf("expected mark b unpredicted; got %+v", r.Skipped.UnpredictedMarks)
	}
	if len(r.Skipped.UnmatchedPredicted) != 1 || r.Skipped.UnmatchedPredicted[0] != "ghost" {
		t.Fatalf("expected ghost unmatched; got %+v", r.Skipped.UnmatchedPredicted)
	}
}
