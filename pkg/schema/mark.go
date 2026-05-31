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
	RDDType       RDDType `json:"rdd_type"`

	// Design fixes the estimand: the running variable, the cutoff, the action
	// and its counterfactual, and the outcome definition.
	Design Design `json:"design"`

	// Context describes the pre-decision state available to a predictor — the
	// covariates the decision-maker observed before acting (pre-treatment only).
	Context Context `json:"context"`

	// Data references the full analysis-ready episode table the RDD is fit on
	// (one row per unit × decision-period, pinned to its decision-time vintage).
	// The table is published as a downloadable artifact (object storage), NOT
	// embedded — keeping the mark itself a small, diffable instrument and the
	// bulky rows in convenient columnar form for model trainers. Integrity is
	// guaranteed by Data.SHA256; the table is also re-derivable byte-for-byte
	// from the frozen inputs + the deterministic build.
	Data DataArtifact `json:"data"`

	// Sample is a small inline excerpt of the episode rows nearest the cutoff,
	// for human inspection/audit without downloading the full table. Optional.
	Sample []Observation `json:"sample,omitempty"`

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

// DataArtifact references the published analysis-ready episode table for a mark.
// It is content-addressed by SHA256 so a consumer can verify the download, and
// carries enough metadata (format, rows, columns) to use the table without the
// mint. URI is the download location (e.g. an R2/object-store URL).
type DataArtifact struct {
	URI     string   `json:"uri"`
	SHA256  string   `json:"sha256"`
	Format  string   `json:"format"` // e.g. "csv.gz", "parquet"
	Rows    int      `json:"rows"`
	Bytes   int64    `json:"bytes,omitempty"`
	Columns []string `json:"columns,omitempty"`
}

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
	case SeriesAreaFunding, SeriesFloorStandards, SeriesSHMI, SeriesBathingWater:
	default:
		return fmt.Errorf("mark %q: unknown series %q", m.ID, m.Series)
	}
	switch m.RDDType {
	case Sharp, Fuzzy:
	default:
		return fmt.Errorf("mark %q: unknown rdd_type %q", m.ID, m.RDDType)
	}
	switch m.Design.Direction {
	case AboveTreated, BelowTreated:
	default:
		return fmt.Errorf("mark %q: unknown direction %q", m.ID, m.Design.Direction)
	}
	if err := m.Effect.Validate(); err != nil {
		return fmt.Errorf("mark %q effect: %w", m.ID, err)
	}
	if m.Effect.Interval == nil {
		return fmt.Errorf("mark %q: effect must carry an honest interval", m.ID)
	}
	// A fuzzy mark's admission depends on a real first stage.
	if m.RDDType == Fuzzy && m.Dossier.FirstStage == nil {
		return fmt.Errorf("mark %q: fuzzy mark requires a first-stage result in its dossier", m.ID)
	}
	if err := m.Provenance.Validate(); err != nil {
		return fmt.Errorf("mark %q provenance: %w", m.ID, err)
	}
	// The external episode table must be referenced and content-addressed.
	if m.Data.URI == "" || m.Data.SHA256 == "" {
		return fmt.Errorf("mark %q: data artifact must have a uri and sha256", m.ID)
	}
	if m.Data.Rows <= 0 {
		return fmt.Errorf("mark %q: data artifact must report a positive row count", m.ID)
	}
	// Assignment consistency of the inline sample rows (if any).
	for i, o := range m.Sample {
		if err := m.checkAssignment(o); err != nil {
			return fmt.Errorf("mark %q sample row %d (%s): %w", m.ID, i, o.UnitID, err)
		}
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
