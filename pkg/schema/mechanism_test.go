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
