package schema

import (
	"errors"
	"fmt"
)

// Mark is one causally-validated reference point on the yardstick.
//
// It is NOT a single number. It is a distribution over the true effect of an
// institutional decision that was (sharply or fuzzily) triggered when a unit's
// running variable crossed a published cutoff. The distribution's width carries
// honest identification uncertainty, and the Mark ships with the validity
// dossier that earned its admission.
//
// A Mark is admitted iff it passes the validity battery (no manipulation, no
// covariate jump, clean placebos, and — for fuzzy seams — a real first stage).
// It is NEVER rejected for a wide interval; width is information.
type Mark struct {
	SchemaVersion string  `json:"schema_version"`
	ID            string  `json:"id"`
	Series        Series  `json:"series"`
	Domain        string  `json:"domain"`
	UnitType      string  `json:"unit_type"`
	RDDType       RDDType `json:"rdd_type,omitempty"`

	// Identification is the design-family discriminator (rdd-sharp / rdd-fuzzy /
	// rdd-kink / did / its-controlled). It selects which Design sub-shape and which
	// Dossier block a reader should expect. Marks minted before this field carry
	// only RDDType; EffectiveIdentification migrates them. New ITS marks set this
	// directly and leave RDDType empty.
	Identification Identification `json:"identification,omitempty"`

	// RowShape declares the shape of this mark's episode rows: cross-section (RDD/
	// DiD, one row per unit) or panel (ITS, one row per series × time bucket). An
	// empty value is derived by EffectiveRowShape (panel for ITS, else cross-section).
	RowShape RowShape `json:"row_shape,omitempty"`

	// MechanismID is the mechanism this mark belongs to — the entity on which
	// identified marks are anchors and bridge marks interpolate. Anchor coherence
	// is defined at the mechanism (shared policy variable / outcome construct /
	// population / regime). Marks grouped under one mechanism with bracketing
	// policy points form an anchor family a bridge can span. See Mechanism.
	MechanismID string `json:"mechanism_id"`

	// Category separates identified (design-based, the pins) from bridge
	// (simulator-bridged interpolation, the span). An empty value reads as
	// `identified` so marks minted before this field keep validating.
	Category Category `json:"category,omitempty"`

	// TruthSource is the hard provenance line: `identified` for design-based
	// marks, `simulator-bridged` for bridge marks. Never aggregated across
	// categories. Empty reads as `identified`.
	TruthSource TruthSource `json:"truth_source,omitempty"`

	// Bridge carries the bridge-specific fields (anchors, query point, simulator,
	// kernel, coherence). Present iff Category is `bridge`; nil for identified.
	Bridge *BridgeSpec `json:"bridge,omitempty"`

	// Design fixes the estimand: the running variable, the cutoff, the action
	// and its counterfactual, and the outcome definition.
	Design Design `json:"design"`

	// Context describes the pre-decision state available to a predictor — the
	// covariates the decision-maker observed before acting (pre-treatment only).
	Context Context `json:"context"`

	// The full analysis-ready rows the RDD is fit on (one row per unit ×
	// decision-period) are NOT embedded here. They live, alongside every other
	// mark's rows, in the single published `episodes` dataset (object storage),
	// where they are recovered by filtering on this mark's ID. Keeping the rows
	// out of the mark keeps the mark a small, diffable metadata instrument; the
	// episodes dataset is re-derivable byte-for-byte from the frozen inputs + the
	// deterministic build. The two stored artifacts are exactly: the marks (here,
	// in git) and the episodes dataset (in object storage), joined on ID.

	// Sample is a small inline excerpt of the episode rows nearest the cutoff,
	// for human inspection/audit without downloading the full table. Optional;
	// cross-section marks only.
	Sample []Observation `json:"sample,omitempty"`

	// PanelSample is the ITS analogue of Sample: a small inline excerpt of the
	// panel episode rows (treated and control series near the intervention
	// instant). Optional; its-controlled marks only.
	PanelSample []PanelObservation `json:"panel_sample,omitempty"`

	// Effect is the mark itself: the honest interval over the true effect at the
	// cutoff (a local-to-cutoff estimand).
	Effect Distribution `json:"effect"`

	// Dossier holds the validity battery results and the admission verdict.
	Dossier ValidityDossier `json:"dossier"`

	// Provenance records sources, licences, point-in-time timestamps, and the
	// determinism record (seeds, tool versions, input hashes).
	Provenance Provenance `json:"provenance"`
}

// Design fixes what is being estimated.
type Design struct {
	// RunningVariable is the continuous quantity assignment depends on.
	RunningVariable Variable `json:"running_variable"`

	// Cutoff is the published threshold c. Direction states which side receives
	// the action.
	Cutoff    float64   `json:"cutoff"`
	Direction Direction `json:"direction"`

	// Action is the policy lever applied when the cutoff is crossed; Alternative
	// is its counterfactual. This action/counterfactual pair is what makes the
	// instrument a *decision* yardstick rather than a pure effect benchmark.
	Action      string `json:"action"`
	Alternative string `json:"alternative"`

	// Outcome is the later, openly-observable quantity measured at the same unit.
	Outcome Variable `json:"outcome"`

	// Estimand names the quantity the Effect distribution describes (e.g. the
	// sharp RD effect / LATE at the cutoff).
	Estimand string `json:"estimand"`

	// PolicySlopeChange is required for a regression-kink design (rdd_type=kink)
	// and is the KNOWN change in the deterministic policy function's slope at the
	// kink, b'(c+) − b'(c−). The RKD estimand is the kink in the outcome's slope
	// divided by this — the marginal effect of the policy intensity. It must be
	// non-zero for a kink, and is absent for sharp/fuzzy level designs.
	PolicySlopeChange *float64 `json:"policy_slope_change,omitempty"`

	// ITS carries the controlled-interrupted-time-series design fields (the
	// intervention instant, pre/post windows, counterfactual model, control
	// series) in place of the RDD-only running_variable/cutoff/direction triplet.
	// Present iff the mark's identification is its-controlled; nil otherwise. The
	// shared fields above (Action, Alternative, Outcome, Estimand) still apply —
	// note an ITS Estimand is a population effect over the post window, not a
	// local-at-cutoff effect, so its scores never pool with RDD marks.
	ITS *ITSDesign `json:"its,omitempty"`
}

// Direction states which side of the cutoff is treated.
type Direction string

const (
	// AboveTreated: units with running variable >= cutoff receive the action.
	AboveTreated Direction = "above-treated"
	// BelowTreated: units with running variable <= cutoff receive the action
	// (e.g. "most deprived" indices where a *lower* rank means more deprived).
	BelowTreated Direction = "below-treated"
)

// Variable describes a named quantity with its units and open-data source.
type Variable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Units       string `json:"units"`
	SourceID    string `json:"source_id"` // references a Provenance.Source.SourceID
}

// Context is the pre-decision setting exposed to a model under test. It must
// contain only information available at or before the decision (pre-treatment).
type Context struct {
	Description string `json:"description"`

	// CovariateNames lists the pre-treatment covariates carried per Observation.
	CovariateNames []string `json:"covariate_names,omitempty"`

	// Population summarises the units in scope (e.g. "317 lower-tier English
	// local authority districts").
	Population string `json:"population,omitempty"`
}

// Observation is one unit's episode: its running-variable value at the
// decision-time vintage, the action it was assigned/received, its later
// outcome, and its pre-treatment covariates. Post-treatment variables must
// never appear in Covariates.
type Observation struct {
	UnitID       string             `json:"unit_id"`
	UnitName     string             `json:"unit_name,omitempty"`
	RunningValue float64            `json:"running_value"`
	Assigned     bool               `json:"assigned"`          // running variable places it on the treated side
	Treated      *bool              `json:"treated,omitempty"` // realized receipt (may differ from Assigned under fuzzy assignment)
	Outcome      *float64           `json:"outcome,omitempty"`
	Covariates   map[string]float64 `json:"covariates,omitempty"`
}

// Validate performs structural and point-in-time integrity checks on a Mark.
// It does NOT re-run the statistical validity battery (that lives in /internal);
// it checks the wire-format invariants any consumer can rely on.
func (m Mark) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("mark %q: schema_version %q != supported %q", m.ID, m.SchemaVersion, SchemaVersion)
	}
	if m.ID == "" {
		return errors.New("mark: empty id")
	}
	switch m.Series {
	case SeriesAreaFunding, SeriesFloorStandards, SeriesSHMI, SeriesBathingWater, SeriesULEZNO2, SeriesBerlinLEZ, SeriesCAMenthol:
	default:
		return fmt.Errorf("mark %q: unknown series %q", m.ID, m.Series)
	}

	// Category and truth_source are normalised: an empty value reads as
	// identified (so marks minted before these fields keep validating). The
	// pin/span discipline is then enforced per category.
	cat := m.EffectiveCategory()
	if err := m.validateProvenanceLine(cat); err != nil {
		return err
	}
	if err := m.validateMechanismID(); err != nil {
		return err
	}
	if err := m.validateIdentificationConsistency(); err != nil {
		return err
	}
	if err := m.validateRowShape(); err != nil {
		return err
	}

	if err := m.Effect.Validate(); err != nil {
		return fmt.Errorf("mark %q effect: %w", m.ID, err)
	}
	if m.Effect.Interval == nil {
		return fmt.Errorf("mark %q: effect must carry an honest interval", m.ID)
	}
	if err := m.Provenance.Validate(); err != nil {
		return fmt.Errorf("mark %q provenance: %w", m.ID, err)
	}

	switch cat {
	case CategoryBridge:
		if err := m.validateBridge(); err != nil {
			return err
		}
	default: // identified
		switch m.EffectiveIdentification() {
		case IDITSControlled:
			if err := m.validateITS(); err != nil {
				return err
			}
		case IDRDDSharp, IDRDDFuzzy, IDRDDKink, IDDiD:
			if err := m.validateRDD(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("mark %q: unknown identification %q (rdd_type %q)", m.ID, m.Identification, m.RDDType)
		}
	}
	return nil
}

// EffectiveIdentification returns the mark's identification family, preferring
// the explicit Identification field and otherwise migrating the legacy RDDType.
// Returns "" for a mark that carries neither a known identification nor a known
// rdd_type — Validate then reports it as unknown.
func (m Mark) EffectiveIdentification() Identification {
	if m.Identification != "" {
		return m.Identification
	}
	return rddToIdentification(m.RDDType)
}

// EffectiveRowShape returns the mark's row shape, deriving an empty value: panel
// for an ITS mark, cross-section for everything else.
func (m Mark) EffectiveRowShape() RowShape {
	if m.RowShape != "" {
		return m.RowShape
	}
	if m.EffectiveIdentification() == IDITSControlled {
		return RowPanel
	}
	return RowCrossSection
}

// validateIdentificationConsistency rejects a mark whose explicit Identification
// contradicts its legacy RDDType (e.g. identification=rdd-fuzzy with
// rdd_type=sharp). When only one is set there is nothing to reconcile.
func (m Mark) validateIdentificationConsistency() error {
	if m.Identification == "" || m.RDDType == "" {
		return nil
	}
	if got := rddToIdentification(m.RDDType); got != m.Identification {
		return fmt.Errorf("mark %q: identification %q contradicts rdd_type %q (which reads as %q)",
			m.ID, m.Identification, m.RDDType, got)
	}
	return nil
}

// validateRowShape rejects an unknown row_shape and an ITS mark that declares the
// cross-section shape (ITS rows are a panel). An empty value is derived and valid.
func (m Mark) validateRowShape() error {
	switch m.RowShape {
	case "", RowCrossSection, RowPanel:
	default:
		return fmt.Errorf("mark %q: unknown row_shape %q", m.ID, m.RowShape)
	}
	if m.EffectiveIdentification() == IDITSControlled && m.RowShape == RowCrossSection {
		return fmt.Errorf("mark %q: an its-controlled mark has panel rows, not cross-section", m.ID)
	}
	return nil
}

// EffectiveCategory returns the mark's category, treating an empty value as
// identified (so marks minted before the field reads as identified). Consumers
// — notably the scorer — use this to keep the two categories strictly separated.
func (m Mark) EffectiveCategory() Category {
	if m.Category == "" {
		return CategoryIdentified
	}
	return m.Category
}

// validateProvenanceLine enforces the hard truth_source line: it must be empty
// (read as identified) or match the category. A bridge mark must never claim
// identified truth, and an identified mark must never claim simulator-bridged.
func (m Mark) validateProvenanceLine(cat Category) error {
	switch m.TruthSource {
	case "":
		// empty reads as identified; only valid for identified marks
		if cat == CategoryBridge {
			return fmt.Errorf("mark %q: bridge mark must set truth_source=%q", m.ID, TruthSimulatorBridged)
		}
	case TruthIdentified:
		if cat == CategoryBridge {
			return fmt.Errorf("mark %q: bridge mark cannot claim truth_source=%q", m.ID, TruthIdentified)
		}
	case TruthSimulatorBridged:
		if cat != CategoryBridge {
			return fmt.Errorf("mark %q: only a bridge mark may set truth_source=%q", m.ID, TruthSimulatorBridged)
		}
	default:
		return fmt.Errorf("mark %q: unknown truth_source %q", m.ID, m.TruthSource)
	}
	return nil
}

// validateRDD checks the invariants specific to a discontinuity/DiD design-based
// mark: a valid direction, a first stage for fuzzy marks, a policy-slope change
// for kink marks, no bridge block, and assignment-consistent sample rows. It
// routes on EffectiveIdentification, so a mark that sets `identification` without
// the legacy `rdd_type` gets the same per-family checks.
func (m Mark) validateRDD() error {
	if m.Bridge != nil {
		return fmt.Errorf("mark %q: identified mark must not carry a bridge block", m.ID)
	}
	id := m.EffectiveIdentification()
	// Direction (which side of a cutoff is treated) is a discontinuity-design
	// concept; a difference-in-differences design has treatment groups, not a
	// cutoff side, so the check applies only to the cutoff designs.
	if id != IDDiD {
		switch m.Design.Direction {
		case AboveTreated, BelowTreated:
		default:
			return fmt.Errorf("mark %q: unknown direction %q", m.ID, m.Design.Direction)
		}
	}
	// A fuzzy mark's admission depends on a real first stage.
	if id == IDRDDFuzzy && m.Dossier.FirstStage == nil {
		return fmt.Errorf("mark %q: fuzzy mark requires a first-stage result in its dossier", m.ID)
	}
	// A kink mark identifies its effect from a known change in the policy slope,
	// so that change must be present and non-zero; a level design must not carry it.
	switch id {
	case IDRDDKink:
		if m.Design.PolicySlopeChange == nil || *m.Design.PolicySlopeChange == 0 {
			return fmt.Errorf("mark %q: kink design requires a non-zero design.policy_slope_change", m.ID)
		}
	default:
		if m.Design.PolicySlopeChange != nil {
			return fmt.Errorf("mark %q: policy_slope_change is only valid for a kink design", m.ID)
		}
	}
	// Assignment consistency of the inline sample rows (if any). This checks each
	// row's treated side against the cutoff, so it applies only to the cutoff
	// designs; a DiD mark's rows are panel observations, not cutoff assignments.
	if id != IDDiD {
		for i, o := range m.Sample {
			if err := m.checkAssignment(o); err != nil {
				return fmt.Errorf("mark %q sample row %d (%s): %w", m.ID, i, o.UnitID, err)
			}
		}
	}
	return nil
}

// validateITS checks the invariants specific to a controlled-interrupted-time-
// series mark: the design.its block with its mandatory fields, a control series
// (an identified ITS mark must be controlled — uncontrolled ITS belongs in a
// bridge), no kink-only slope change, no bridge block, and panel sample rows
// whose post flag agrees with their signed distance from the intervention.
func (m Mark) validateITS() error {
	if m.Bridge != nil {
		return fmt.Errorf("mark %q: identified mark must not carry a bridge block", m.ID)
	}
	if m.Design.PolicySlopeChange != nil {
		return fmt.Errorf("mark %q: policy_slope_change is only valid for a kink design", m.ID)
	}
	d := m.Design.ITS
	if d == nil {
		return fmt.Errorf("mark %q: its-controlled mark requires a design.its block", m.ID)
	}
	if d.InterventionInstant == "" {
		return fmt.Errorf("mark %q: its mark requires design.its.intervention_instant", m.ID)
	}
	if d.PreWindow.Start == "" || d.PreWindow.End == "" {
		return fmt.Errorf("mark %q: its mark requires a pre_window with start and end", m.ID)
	}
	if d.PostWindow.Start == "" || d.PostWindow.End == "" {
		return fmt.Errorf("mark %q: its mark requires a post_window with start and end", m.ID)
	}
	if d.Counterfactual.Family == "" {
		return fmt.Errorf("mark %q: its mark requires a counterfactual.family", m.ID)
	}
	if d.Control == nil {
		return fmt.Errorf("mark %q: an identified its-controlled mark requires a control series (uncontrolled ITS should be a bridge)", m.ID)
	}
	for i, o := range m.PanelSample {
		if o.IsPost && o.PeriodsSinceIntervention < 0 {
			return fmt.Errorf("mark %q panel sample row %d (%s): is_post=true but periods_since_intervention=%g is pre-intervention",
				m.ID, i, o.SeriesID, o.PeriodsSinceIntervention)
		}
	}
	return nil
}

// validateBridge enforces the bridge data-model invariants: a bridge block, >=2
// anchors, a mandatory coherence justification, and — the load-bearing one —
// that the query point is strictly bracketed by anchors on the policy variable.
// Bracketing is enforced HERE (in the data model), not left to dossier
// discretion: there is no extrapolation path to fall back to.
func (m Mark) validateBridge() error {
	b := m.Bridge
	if b == nil {
		return fmt.Errorf("mark %q: bridge mark requires a bridge block", m.ID)
	}
	if b.Mechanism == "" {
		return fmt.Errorf("mark %q: bridge mark requires a mechanism id", m.ID)
	}
	if len(b.Anchors) < 2 {
		return fmt.Errorf("mark %q: bridge mark requires >=2 anchors (got %d)", m.ID, len(b.Anchors))
	}
	if b.AnchorCoherence.Justification == "" {
		return fmt.Errorf("mark %q: bridge mark requires an anchor-coherence justification", m.ID)
	}
	// Bracketing: at least one anchor strictly below and one strictly above the
	// query point on the policy variable (interpolation only).
	var below, above bool
	for _, a := range b.Anchors {
		if a.MarkID == "" {
			return fmt.Errorf("mark %q: bridge anchor has empty mark_id", m.ID)
		}
		switch {
		case a.PolicyPoint < b.QueryPoint:
			below = true
		case a.PolicyPoint > b.QueryPoint:
			above = true
		}
	}
	if !below || !above {
		return fmt.Errorf("mark %q: query_point %g is not strictly bracketed by anchors (below=%v above=%v); extrapolation is out of scope",
			m.ID, b.QueryPoint, below, above)
	}
	return nil
}

// checkAssignment verifies the Assigned flag is monotone-consistent with the
// running value, cutoff, and direction — a cheap guard against a loader
// mislabelling the treated side. It checks only the *strict* sides, leaving the
// exact-cutoff boundary convention to the series (e.g. the floor standard treats
// P8 strictly below -0.5, so a school exactly at -0.5 is a control even though
// it sits on the cutoff).
func (m Mark) checkAssignment(o Observation) error {
	var bad bool
	switch m.Design.Direction {
	case AboveTreated:
		// treated must not lie strictly below; control must not lie strictly above.
		bad = (o.Assigned && o.RunningValue < m.Design.Cutoff) ||
			(!o.Assigned && o.RunningValue > m.Design.Cutoff)
	case BelowTreated:
		// treated must not lie strictly above; control must not lie strictly below.
		bad = (o.Assigned && o.RunningValue > m.Design.Cutoff) ||
			(!o.Assigned && o.RunningValue < m.Design.Cutoff)
	}
	if bad {
		return fmt.Errorf("assigned=%v inconsistent with running=%g, cutoff=%g, direction=%s",
			o.Assigned, o.RunningValue, m.Design.Cutoff, m.Design.Direction)
	}
	return nil
}
