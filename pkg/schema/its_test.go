package schema

import "testing"

// validITSMark builds a well-formed controlled-interrupted-time-series mark,
// reusing the floor-standards mechanism/series the other tests use (validation
// does not tie ITS to a particular series).
func validITSMark() Mark {
	m := validMark()
	m.RDDType = ""
	m.Identification = IDITSControlled
	m.Design = Design{
		Action:      "switch on",
		Alternative: "leave off",
		Outcome:     Variable{Name: "deaths", Units: "per 100k"},
		Estimand:    "population effect over the post window",
		ITS: &ITSDesign{
			InterventionInstant: "2018-05-01",
			RunningTime:         Variable{Name: "month", Units: "month"},
			PreWindow:           Window{Start: "2014-01", End: "2018-04"},
			PostWindow:          Window{Start: "2018-05", End: "2020-12"},
			Counterfactual:      Counterfactual{Family: "segmented-regression", Justification: "linear pre-trend with monthly seasonality"},
			Control:             &ControlSeries{SeriesID: "imd2019", Role: "parallel-trend", Justification: "shares pre-trend"},
		},
	}
	m.Dossier = ValidityDossier{Admitted: true, ITS: &ITSChecks{Admitted: true}}
	return m
}

// TestITSMarkValid confirms a complete ITS mark validates and that its
// identification and row shape resolve as expected.
func TestITSMarkValid(t *testing.T) {
	m := validITSMark()
	if err := m.Validate(); err != nil {
		t.Fatalf("valid ITS mark rejected: %v", err)
	}
	if m.EffectiveIdentification() != IDITSControlled {
		t.Errorf("EffectiveIdentification = %q, want %q", m.EffectiveIdentification(), IDITSControlled)
	}
	if m.EffectiveRowShape() != RowPanel {
		t.Errorf("EffectiveRowShape = %q, want panel", m.EffectiveRowShape())
	}
}

// TestITSRequiredFields checks each mandatory ITS design field is enforced.
func TestITSRequiredFields(t *testing.T) {
	cases := map[string]func(*Mark){
		"missing its block":            func(m *Mark) { m.Design.ITS = nil },
		"missing intervention":         func(m *Mark) { m.Design.ITS.InterventionInstant = "" },
		"missing pre-window":           func(m *Mark) { m.Design.ITS.PreWindow = Window{} },
		"missing post-window":          func(m *Mark) { m.Design.ITS.PostWindow = Window{} },
		"missing counterfactual":       func(m *Mark) { m.Design.ITS.Counterfactual.Family = "" },
		"missing control (identified)": func(m *Mark) { m.Design.ITS.Control = nil },
	}
	for name, mutate := range cases {
		m := validITSMark()
		mutate(&m)
		if m.Validate() == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

// TestITSForbidsKinkSlope confirms an ITS mark may not carry the kink-only
// policy_slope_change.
func TestITSForbidsKinkSlope(t *testing.T) {
	s := 0.1
	m := validITSMark()
	m.Design.PolicySlopeChange = &s
	if m.Validate() == nil {
		t.Error("an ITS mark must not carry policy_slope_change (kink-only)")
	}
}

// TestITSPanelRowConsistency checks the panel sample guard: a post row cannot sit
// at a pre-intervention distance.
func TestITSPanelRowConsistency(t *testing.T) {
	m := validITSMark()
	m.PanelSample = []PanelObservation{
		{SeriesID: "treated", Period: "2018-06", PeriodsSinceIntervention: 1, IsPost: true},
		{SeriesID: "treated", Period: "2018-03", PeriodsSinceIntervention: -2, IsPost: false},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("consistent panel rows rejected: %v", err)
	}
	m.PanelSample[1].IsPost = true // post but pre-intervention distance
	if m.Validate() == nil {
		t.Error("expected rejection of a post row at a pre-intervention distance")
	}
}

// TestITSRowShapeConsistency checks an ITS mark may not declare the cross-section
// row shape.
func TestITSRowShapeConsistency(t *testing.T) {
	m := validITSMark()
	m.RowShape = RowCrossSection
	if m.Validate() == nil {
		t.Error("an ITS mark must not declare row_shape=cross-section")
	}
	m.RowShape = RowPanel
	if err := m.Validate(); err != nil {
		t.Errorf("ITS mark with explicit panel row_shape rejected: %v", err)
	}
}

// TestIdentificationMigration confirms the legacy rdd_type discriminator migrates
// to the forward identification value, and that a contradiction is rejected.
func TestIdentificationMigration(t *testing.T) {
	for rdd, want := range map[RDDType]Identification{
		Sharp: IDRDDSharp, Fuzzy: IDRDDFuzzy, Kink: IDRDDKink, DiD: IDDiD,
	} {
		m := Mark{RDDType: rdd}
		if got := m.EffectiveIdentification(); got != want {
			t.Errorf("rdd_type %q migrated to %q, want %q", rdd, got, want)
		}
	}

	// A sharp mark (default validMark) with a contradictory identification is rejected.
	m := validMark()
	m.Identification = IDRDDFuzzy
	if m.Validate() == nil {
		t.Error("identification contradicting rdd_type should be rejected")
	}

	// Setting identification consistently with rdd_type is fine.
	m = validMark()
	m.Identification = IDRDDSharp
	if err := m.Validate(); err != nil {
		t.Errorf("consistent identification+rdd_type rejected: %v", err)
	}
}
