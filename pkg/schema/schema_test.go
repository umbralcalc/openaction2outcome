package schema

import "testing"

func goodProvenance() Provenance {
	return Provenance{
		Sources: []Source{{
			SourceID: "imd2019", Title: "IoD2019", Publisher: "MHCLG",
			DownloadURI: "https://example/file.xlsx", RetrievedAt: "2026-05-30",
			Licence: "OGL v3.0", SHA256: "abc123", Vintage: "IoD2019",
		}},
		ContextAsOf:            "2019-09-26",
		DecisionTimestamp:      "2025-03-01",
		OutcomeTimestamp:       "2027-03-01",
		RunningVariableVintage: "IoD2019",
		OutcomeRealized:        true,
	}
}

func TestProvenancePointInTime(t *testing.T) {
	p := goodProvenance()
	if err := p.Validate(); err != nil {
		t.Fatalf("good provenance rejected: %v", err)
	}

	// decision before context -> leakage.
	bad := goodProvenance()
	bad.ContextAsOf = "2025-06-01"
	if bad.Validate() == nil {
		t.Fatal("expected rejection when context_asof > decision_timestamp")
	}

	// outcome not strictly after decision -> leakage.
	bad = goodProvenance()
	bad.OutcomeTimestamp = "2025-03-01"
	if bad.Validate() == nil {
		t.Fatal("expected rejection when decision_timestamp >= outcome_timestamp")
	}

	// claiming a realized outcome without an outcome timestamp.
	bad = goodProvenance()
	bad.OutcomeTimestamp = ""
	if bad.Validate() == nil {
		t.Fatal("expected rejection when outcome_realized but no outcome_timestamp")
	}

	// a pending-outcome mark (no timestamp, not realized) is allowed.
	pending := goodProvenance()
	pending.OutcomeTimestamp = ""
	pending.OutcomeRealized = false
	if err := pending.Validate(); err != nil {
		t.Fatalf("pending-outcome provenance should be allowed: %v", err)
	}
}

func TestMarkAssignmentConsistency(t *testing.T) {
	m := Mark{
		SchemaVersion: SchemaVersion,
		ID:            "t1",
		Seam:          SeamAreaFunding,
		RDDType:       Sharp,
		Design:        Design{Cutoff: 20, Direction: BelowTreated}, // most-deprived = low percentile
		Effect: Distribution{
			Central:  0.5,
			Interval: &Interval{Level: 0.95, Lower: 0.3, Upper: 0.7},
		},
		Provenance: goodProvenance(),
		Data:       DataArtifact{URI: "https://example/episodes.csv.gz", SHA256: "deadbeef", Format: "csv.gz", Rows: 2},
		Sample: []Observation{
			{UnitID: "la1", RunningValue: 12, Assigned: true},  // below cutoff -> treated
			{UnitID: "la2", RunningValue: 55, Assigned: false}, // above cutoff -> control
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("consistent mark rejected: %v", err)
	}

	m.Sample[0].Assigned = false // now inconsistent with running<cutoff
	if m.Validate() == nil {
		t.Fatal("expected rejection of mislabelled assignment")
	}
}
