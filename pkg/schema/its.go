package schema

// ITSDesign carries the design fields specific to a controlled interrupted time
// series. It sits inside Design as the time-domain analogue of the RDD-only
// running_variable/cutoff/direction triplet: the forcing variable is time, the
// "cutoff" is a sharp intervention instant, and comparability is established by a
// control series sharing the treated series' pre-intervention trend.
//
// The shared design fields (Action, Alternative, Outcome, Estimand) stay on the
// parent Design; only the ITS-specific fields live here. Present iff the mark's
// identification is its-controlled.
type ITSDesign struct {
	// InterventionInstant is the sharp date/time the action took effect (ISO 8601)
	// — the time-domain analogue of Design.Cutoff.
	InterventionInstant string `json:"intervention_instant"`

	// RunningTime describes the time axis (e.g. units "month"). The forcing
	// variable is time; it is recorded for symmetry with the RDD running variable.
	RunningTime Variable `json:"running_time"`

	// PreWindow is the pre-intervention period used to fit the counterfactual;
	// PostWindow is the period over which the effect is accumulated/averaged.
	PreWindow  Window `json:"pre_window"`
	PostWindow Window `json:"post_window"`

	// Transition is an implementation ramp to exclude from both windows — the
	// time-domain analogue of the RDD donut. Nil when there is no ramp.
	Transition *Window `json:"transition,omitempty"`

	// Counterfactual is the model of what the treated series would have done absent
	// the action (e.g. segmented regression with seasonal terms). It is the single
	// biggest specification choice and is recorded openly.
	Counterfactual Counterfactual `json:"counterfactual"`

	// Control is the comparison series (e.g. England for a Scotland intervention).
	// Omit only for uncontrolled ITS, which is discouraged and should usually be a
	// bridge rather than an identified mark — so an identified ITS mark requires it.
	Control *ControlSeries `json:"control,omitempty"`
}

// Window is a closed [Start, End] period in the running_time units (ISO 8601).
type Window struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Counterfactual is the segmented model of the treated series absent the action.
type Counterfactual struct {
	// Family is the model family (e.g. "segmented-regression", "negative-binomial
	// GLM with linear time + seasonal terms").
	Family string `json:"family"`

	// Terms lists the regression terms entering the counterfactual (e.g.
	// ["intercept", "linear-time", "month-of-year"]).
	Terms []string `json:"terms,omitempty"`

	// Seasonality describes how seasonality is modelled (e.g. "monthly dummies",
	// "harmonic, 2 pairs"); empty when the series carries no seasonality.
	Seasonality string `json:"seasonality,omitempty"`

	// Justification is the plain-language argument for this specification — the
	// most consequential ITS choice, so it is mandatory.
	Justification string `json:"justification"`
}

// ControlSeries names the comparison series and its comparability role.
type ControlSeries struct {
	SeriesID string `json:"series_id"` // references a Provenance.Source.SourceID
	// Role states why the series is comparable; e.g. "parallel-trend".
	Role          string `json:"role"`
	Justification string `json:"justification"`
}

// PanelObservation is one (series × time bucket) episode row — the ITS row shape.
// It is the panel analogue of the cross-section Observation: instead of a unit's
// running-variable value and assignment side, it carries which series the row
// belongs to (treated or a control), the time bucket, the signed distance from
// the intervention instant (the analogue of distance_to_cutoff), whether the
// bucket is post-intervention, and the observed outcome. Everything constant for
// the mark lives in the mark JSON and is joined on the mark id.
type PanelObservation struct {
	SeriesID   string `json:"series_id"`
	SeriesName string `json:"series_name,omitempty"`

	// IsControl is true for control-series rows.
	IsControl bool `json:"is_control"`

	// Period is the time bucket (ISO 8601, e.g. "2018-05").
	Period string `json:"period"`

	// PeriodsSinceIntervention is period − intervention_instant in
	// running_time.units; negative is pre-intervention. The analogue of
	// distance_to_cutoff; derived, exactly as distance_to_cutoff is.
	PeriodsSinceIntervention float64 `json:"periods_since_intervention"`

	// IsPost is true when the bucket is on/after the intervention instant and
	// outside any transition ramp.
	IsPost bool `json:"is_post"`

	// Outcome is the observed outcome value in that period for that series; nil
	// when missing (e.g. a gap in the series).
	Outcome *float64 `json:"outcome,omitempty"`

	// Covariates carries one entry per Context.CovariateNames (e.g. a population
	// denominator or seasonal index). Post-treatment variables must never appear.
	Covariates map[string]float64 `json:"covariates,omitempty"`
}
