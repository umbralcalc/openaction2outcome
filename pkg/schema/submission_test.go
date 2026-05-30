package schema

import "testing"

func goodSubmission() Submission {
	return Submission{
		SchemaVersion: SchemaVersion,
		ModelName:     "m",
		Predictions: []Prediction{
			{MarkID: "a", Effect: Distribution{Central: 0.1, Interval: &Interval{Level: 0.95, Lower: 0, Upper: 0.2}}},
		},
	}
}

func TestSubmissionValidate(t *testing.T) {
	if err := goodSubmission().Validate(); err != nil {
		t.Fatalf("good submission rejected: %v", err)
	}

	bad := goodSubmission()
	bad.SchemaVersion = "0.0.1"
	mustRejectSub(t, bad, "wrong version")

	bad = goodSubmission()
	bad.ModelName = ""
	mustRejectSub(t, bad, "empty model name")

	bad = goodSubmission()
	bad.Predictions = nil
	mustRejectSub(t, bad, "no predictions")

	bad = goodSubmission()
	bad.Predictions = append(bad.Predictions, bad.Predictions[0])
	mustRejectSub(t, bad, "duplicate mark id")

	bad = goodSubmission()
	bad.Predictions[0].MarkID = ""
	mustRejectSub(t, bad, "empty mark id")

	bad = goodSubmission()
	bad.Predictions[0].Effect.Interval = &Interval{Level: 0.95, Lower: 0.5, Upper: 0.1} // lower>upper
	mustRejectSub(t, bad, "invalid effect distribution")
}

func mustRejectSub(t *testing.T, s Submission, why string) {
	t.Helper()
	if err := s.Validate(); err == nil {
		t.Errorf("expected rejection: %s", why)
	}
}
