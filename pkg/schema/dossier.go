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
