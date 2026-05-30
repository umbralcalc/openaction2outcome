package score

import (
	"math"
	"strings"
	"testing"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

func TestCdfEvalGaussian(t *testing.T) {
	d := schema.Distribution{Central: 1.0, StdDev: ptr(0.5), Interval: &schema.Interval{Level: 0.95, Lower: 0, Upper: 2}}
	mid, ok := cdfEval(d, 1.0)
	if !ok || math.Abs(mid-0.5) > 1e-6 {
		t.Fatalf("CDF at the mean should be 0.5; got %v ok=%v", mid, ok)
	}
	lo, _ := cdfEval(d, -2)
	hi, _ := cdfEval(d, 4)
	if !(lo < mid && mid < hi) {
		t.Fatalf("CDF must increase: %v < %v < %v", lo, mid, hi)
	}
}

func TestCdfEvalSamplesAndQuantiles(t *testing.T) {
	samp := schema.Distribution{Central: 0, Samples: []float64{-2, -1, 0, 1, 2}}
	if c, ok := cdfEval(samp, 0); !ok || math.Abs(c-0.5) > 0.21 {
		t.Fatalf("sample CDF at median ~0.5; got %v ok=%v", c, ok)
	}
	q := schema.Distribution{Central: 0, Quantiles: []schema.Quantile{{P: 0.1, Value: -1}, {P: 0.5, Value: 0}, {P: 0.9, Value: 1}}}
	c1, _ := cdfEval(q, -1)
	c2, _ := cdfEval(q, 1)
	if !(c1 < c2) {
		t.Fatalf("quantile CDF must increase: %v then %v", c1, c2)
	}
}

func TestImpliedStdDevFromInterval(t *testing.T) {
	// Interval only (no StdDev): half-width / z should recover the sd.
	d := schema.Distribution{Central: 0, Interval: &schema.Interval{Level: 0.95, Lower: -1.96, Upper: 1.96}}
	sd, ok := impliedStdDev(d)
	if !ok || math.Abs(sd-1.0) > 1e-2 {
		t.Fatalf("implied sd from a 95%% interval of ±1.96 should be ~1; got %v ok=%v", sd, ok)
	}
}

func TestCramerSelfDistanceZero(t *testing.T) {
	d := schema.Distribution{Central: 0, StdDev: ptr(0.5), Interval: &schema.Interval{Level: 0.95, Lower: -1, Upper: 1}}
	if got := cramerDistance(d, d); got > 1e-6 {
		t.Fatalf("Cramér distance of a distribution with itself should be ~0; got %g", got)
	}
}

func TestReportSummaryAndString(t *testing.T) {
	marks := []schema.Mark{gaussianMark("a", 1, 0.2), gaussianMark("b", -1, 0.2)}
	sub := schema.Submission{
		SchemaVersion: schema.SchemaVersion,
		ModelName:     "m",
		Predictions: []schema.Prediction{
			{MarkID: "a", Effect: pred(1, 0.2).Effect},   // correct sign
			{MarkID: "b", Effect: pred(0.5, 0.2).Effect}, // wrong sign on b
		},
	}
	r := ScoreSubmission(marks, sub, Options{})
	if r.Summary.NumMarksScored != 2 {
		t.Fatalf("expected 2 scored; got %d", r.Summary.NumMarksScored)
	}
	if r.Summary.NumSignKnown != 2 || math.Abs(r.Summary.SignAccuracy-0.5) > 1e-9 {
		t.Fatalf("expected 1/2 signs correct; got known=%d acc=%v", r.Summary.NumSignKnown, r.Summary.SignAccuracy)
	}
	if r.Summary.TotalRegret <= 0 {
		t.Fatalf("a wrong-sign call should accrue regret; got %v", r.Summary.TotalRegret)
	}
	if len(r.Summary.Calibration) != 9 {
		t.Fatalf("calibration curve should have 9 nominal points; got %d", len(r.Summary.Calibration))
	}
	s := r.String()
	if !strings.Contains(s, "Decision:") || !strings.Contains(s, "Calibration:") {
		t.Fatalf("report string should mention both scores:\n%s", s)
	}
}
