package schema

import "testing"

// TestCanonicalMechanismsWellFormed checks every registered mechanism is fully
// specified (id matches its key, all coherence fields present) — the data model's
// mechanism entities are complete.
func TestCanonicalMechanismsWellFormed(t *testing.T) {
	for key, m := range CanonicalMechanisms() {
		if m.ID != key {
			t.Errorf("mechanism %q: ID field %q does not match its key", key, m.ID)
		}
		if m.Name == "" || m.Domain == "" || m.PolicyVariable == "" ||
			m.OutcomeConstruct == "" || m.PopulationDefinition == "" || m.Regime == "" {
			t.Errorf("mechanism %q: missing a required descriptor/coherence field: %+v", key, m)
		}
	}
}

// TestMechanismIDRequiredAndKnown confirms a mark must carry a known mechanism_id.
func TestMechanismIDRequiredAndKnown(t *testing.T) {
	if err := validMark().Validate(); err != nil {
		t.Fatalf("valid mark rejected: %v", err)
	}
	missing := validMark()
	missing.MechanismID = ""
	if missing.Validate() == nil {
		t.Error("a mark with no mechanism_id should be rejected")
	}
	unknown := validMark()
	unknown.MechanismID = "no-such-mechanism"
	if unknown.Validate() == nil {
		t.Error("a mark with an unknown mechanism_id should be rejected")
	}
}

// TestKinkDesignValidation checks the regression-kink design's invariants: a kink
// mark needs a non-zero policy_slope_change; a level (sharp/fuzzy) mark must not
// carry one.
func TestKinkDesignValidation(t *testing.T) {
	s := 1.0 / 3000.0

	ok := validMark()
	ok.RDDType = Kink
	ok.Design.PolicySlopeChange = &s
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid kink mark rejected: %v", err)
	}

	missing := validMark()
	missing.RDDType = Kink
	if missing.Validate() == nil {
		t.Error("a kink mark without policy_slope_change should be rejected")
	}

	zero := 0.0
	zeroSlope := validMark()
	zeroSlope.RDDType = Kink
	zeroSlope.Design.PolicySlopeChange = &zero
	if zeroSlope.Validate() == nil {
		t.Error("a kink mark with zero policy_slope_change should be rejected")
	}

	misplaced := validMark() // sharp by default
	misplaced.Design.PolicySlopeChange = &s
	if misplaced.Validate() == nil {
		t.Error("a level design must not carry policy_slope_change")
	}
}
