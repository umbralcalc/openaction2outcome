package schema

// ValidityDossier ships inside every Mark. It records the result of each
// validity test and the admission verdict. Admission requires passing the
// validity tests; it is never withheld for a wide interval.
type ValidityDossier struct {
	// Density is the manipulation / sorting test at the cutoff (McCrary-style).
	Density TestResult `json:"density"`

	// CovariateContinuity holds one continuity-at-the-cutoff test per
	// pre-treatment covariate (covariates should not jump at c).
	CovariateContinuity []NamedTestResult `json:"covariate_continuity,omitempty"`

	// PlaceboCutoffs are RD estimates at false cutoffs away from c; these should
	// be indistinguishable from zero.
	PlaceboCutoffs []PlaceboResult `json:"placebo_cutoffs,omitempty"`

	// BandwidthSweep records the point estimate across bandwidths; its spread is
	// part of the identification uncertainty folded into the mark's interval.
	BandwidthSweep []SweepPoint `json:"bandwidth_sweep,omitempty"`

	// DonutRobustness re-estimates after excluding units within a radius of c
	// (guards against heaping / exact-cutoff anomalies).
	DonutRobustness []SweepPoint `json:"donut_robustness,omitempty"`

	// FirstStage is required for fuzzy marks: the discontinuity in treatment
	// probability at the cutoff. Nil for sharp marks.
	FirstStage *FirstStageResult `json:"first_stage,omitempty"`

	// Bridge holds the bridge-specific validity battery (anchor coherence, LOAO,
	// kernel sensitivity, bracketing). Present iff the mark's category is bridge;
	// nil for identified marks. It is pure description — the computation lives in
	// internal/bridge — so the schema stays dependency-light.
	Bridge *BridgeChecks `json:"bridge,omitempty"`

	// ITS holds the controlled-interrupted-time-series validity battery (the
	// time-domain analogue of the RDD checks above). Present iff the mark's
	// identification is its-controlled; nil otherwise.
	ITS *ITSChecks `json:"its,omitempty"`

	// SeamSpecificChecks holds validity checks unique to a particular series that
	// fall outside the standard battery — e.g. the bathing-water abnormal-sample-
	// exclusion sensitivity (the analogue of a manipulation check, since extreme-
	// rainfall samples are discounted from the classification by a discretionary
	// rule). Optional; empty for series without one.
	SeamSpecificChecks []NamedTestResult `json:"seam_specific_checks,omitempty"`

	// Admitted is the overall verdict; Notes explains it.
	Admitted bool   `json:"admitted"`
	Notes    string `json:"notes,omitempty"`
}

// TestResult is a generic structured outcome of a validity test.
type TestResult struct {
	Method    string   `json:"method"`
	Statistic *float64 `json:"statistic,omitempty"`
	PValue    *float64 `json:"p_value,omitempty"`
	Passed    bool     `json:"passed"`
	Detail    string   `json:"detail,omitempty"`
}

// NamedTestResult is a TestResult tagged with the covariate it concerns.
type NamedTestResult struct {
	Name string `json:"name"`
	TestResult
}

// PlaceboResult is an RD estimate at a placebo cutoff.
type PlaceboResult struct {
	Cutoff   float64  `json:"cutoff"`
	Estimate float64  `json:"estimate"`
	StdErr   *float64 `json:"std_err,omitempty"`
	Passed   bool     `json:"passed"` // true when indistinguishable from zero
}

// SweepPoint is a single (parameter, estimate) point of a robustness sweep.
type SweepPoint struct {
	Param    float64  `json:"param"` // bandwidth, or donut radius
	Estimate float64  `json:"estimate"`
	StdErr   *float64 `json:"std_err,omitempty"`
}

// FirstStageResult is the fuzzy-series first stage: the jump in treatment
// probability at the cutoff. A fuzzy mark is only admissible with a real jump.
type FirstStageResult struct {
	Jump   float64  `json:"jump"`
	StdErr *float64 `json:"std_err,omitempty"`
	FStat  *float64 `json:"f_stat,omitempty"`
	Passed bool     `json:"passed"`
}

// BridgeChecks is the bridge-specific validity battery, the analogue of the
// manipulation check for identified marks. Everything here is description; the
// computation lives in internal/bridge and internal/validity.
type BridgeChecks struct {
	// Coherence echoes the structured anchor-coherence justification (mandatory).
	Coherence AnchorCoherence `json:"anchor_coherence"`

	// BracketingOK records that the query point lies strictly between anchors on
	// the policy variable (also enforced in Mark.Validate).
	BracketingOK bool `json:"bracketing_ok"`

	// LOAOCoverage is the headline leave-one-anchor-out coverage: the fraction of
	// held-out anchors whose own identified posterior fell within the bridge's
	// predicted interval. This is the bridge analogue of the identified marks'
	// calibration study.
	LOAOCoverage float64   `json:"loao_coverage"`
	LOAOLevel    float64   `json:"loao_level"`
	LOAO         []LOAORow `json:"loao,omitempty"`

	// KernelSensitivity reports how much τ(query) and its interval move under
	// alternative covariance kernels; Flagged is set when the movement is large
	// enough that the estimate is kernel-driven.
	KernelSensitivity []KernelSensitivityRow `json:"kernel_sensitivity,omitempty"`
	KernelFlagged     bool                   `json:"kernel_flagged"`

	// Inference is the deterministic causal layer's record of the rung used and the
	// tractability-gate verdict — the Axis-B check that a deterministic interval is
	// honest for this mechanism. Present once that layer mints the mark.
	Inference *InferenceRecord `json:"inference,omitempty"`

	// Admitted is the bridge verdict; Notes explains it.
	Admitted bool   `json:"admitted"`
	Notes    string `json:"notes,omitempty"`
}

// LOAORow is one leave-one-anchor-out trial: the held-out anchor, the interval
// the bridge predicted for its position, and whether the anchor's own posterior
// was covered. Endpoint anchors cannot be held out without breaking bracketing
// and are reported as skipped.
type LOAORow struct {
	HeldMarkID    string  `json:"held_mark_id"`
	PolicyPoint   float64 `json:"policy_point"`
	AnchorCentral float64 `json:"anchor_central"`
	PredLower     float64 `json:"pred_lower"`
	PredUpper     float64 `json:"pred_upper"`
	Covered       bool    `json:"covered"`
	Skipped       bool    `json:"skipped,omitempty"` // true for endpoint anchors
	SkipReason    string  `json:"skip_reason,omitempty"`
}

// KernelSensitivityRow is τ(query) and its interval under one covariance kernel.
type KernelSensitivityRow struct {
	Kernel  string  `json:"kernel"`
	Central float64 `json:"central"`
	Lower   float64 `json:"lower"`
	Upper   float64 `json:"upper"`
}

// ITSChecks is the controlled-interrupted-time-series validity battery. Each
// check carries the same epistemic intent as a named RDD check; the mark is
// admitted only if they pass, and a wide interval is never itself a failure.
// Everything here is description — the computation would live in internal/its.
type ITSChecks struct {
	// NoAnticipation tests for a pre-trend break or forestalling before the
	// intervention instant (mirrors the RDD density / no-sorting test).
	NoAnticipation TestResult `json:"no_anticipation"`

	// ControlParallelism tests that treated and control share a pre-intervention
	// trend (mirrors covariate_continuity).
	ControlParallelism TestResult `json:"control_parallelism"`

	// PlaceboDates are effect estimates at fake intervention dates in the
	// pre-period; these should be indistinguishable from zero (mirrors
	// placebo_cutoffs in the time axis).
	PlaceboDates []DatePlaceboResult `json:"placebo_dates,omitempty"`

	// PlaceboOutcomes are effect estimates on logically unaffected outcomes; each
	// should show no effect (mirrors placebo_cutoffs, second axis).
	PlaceboOutcomes []NamedTestResult `json:"placebo_outcomes,omitempty"`

	// WindowSweep records estimate stability as the pre/post window lengths vary
	// (mirrors bandwidth_sweep). Param is the window length in running_time units.
	WindowSweep []SweepPoint `json:"window_sweep,omitempty"`

	// TransitionExclusion re-estimates after dropping the implementation ramp
	// (mirrors donut_robustness).
	TransitionExclusion []SweepPoint `json:"transition_exclusion,omitempty"`

	// DoseCheck confirms the action was actually delivered (e.g. sales/price/
	// compliance moved at the date) — the ITS analogue of the fuzzy first stage.
	DoseCheck *FirstStageResult `json:"dose_check,omitempty"`

	// Autocorrelation records that residual serial correlation was modelled
	// (Newey-West / ARMA errors). ITS-specific; it has no RDD analogue.
	Autocorrelation TestResult `json:"autocorrelation"`

	// Admitted is the ITS verdict; Notes explains it.
	Admitted bool   `json:"admitted"`
	Notes    string `json:"notes,omitempty"`
}

// DatePlaceboResult is an ITS effect estimate at a placebo (fake) intervention
// date — the date-keyed analogue of PlaceboResult, whose key is a numeric cutoff.
type DatePlaceboResult struct {
	Date     string   `json:"date"`
	Estimate float64  `json:"estimate"`
	StdErr   *float64 `json:"std_err,omitempty"`
	Passed   bool     `json:"passed"` // true when indistinguishable from zero
}
