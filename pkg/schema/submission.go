package schema

import (
	"errors"
	"fmt"
)

// Submission is what a model under test produces: for each mark it is scored
// on, a predicted effect carrying the model's *own* uncertainty (BRIEF §4).
// The evaluator compares each Prediction's Distribution against the matching
// Mark's honest interval on Tracks A and B.
type Submission struct {
	SchemaVersion string       `json:"schema_version"`
	ModelName     string       `json:"model_name"`
	ModelDetail   string       `json:"model_detail,omitempty"`
	Predictions   []Prediction `json:"predictions"`
}

// Prediction is a model's claim about a single mark.
type Prediction struct {
	MarkID string `json:"mark_id"`

	// Effect is the model's predicted effect (value(action) - value(alternative)
	// at the cutoff) with its own uncertainty. Scored on Track B and, via its
	// sign, on Track A.
	Effect Distribution `json:"effect"`

	// ValueAction and ValueAlternative are optional explicit decision values; if
	// supplied they let Track A score decision regret directly rather than via
	// the effect sign alone.
	ValueAction      *float64 `json:"value_action,omitempty"`
	ValueAlternative *float64 `json:"value_alternative,omitempty"`
}

// Validate checks structural invariants of a submission.
func (s Submission) Validate() error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("submission: schema_version %q != supported %q", s.SchemaVersion, SchemaVersion)
	}
	if s.ModelName == "" {
		return errors.New("submission: empty model_name")
	}
	if len(s.Predictions) == 0 {
		return errors.New("submission: no predictions")
	}
	seen := make(map[string]bool, len(s.Predictions))
	for i, p := range s.Predictions {
		if p.MarkID == "" {
			return fmt.Errorf("submission: prediction %d has empty mark_id", i)
		}
		if seen[p.MarkID] {
			return fmt.Errorf("submission: duplicate prediction for mark %q", p.MarkID)
		}
		seen[p.MarkID] = true
		if err := p.Effect.Validate(); err != nil {
			return fmt.Errorf("submission: prediction for mark %q: %w", p.MarkID, err)
		}
	}
	return nil
}
