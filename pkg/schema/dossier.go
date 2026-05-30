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
