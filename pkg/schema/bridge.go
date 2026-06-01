package schema

// This file adds the bridge-mark category to the schema. It is pure description:
// it states what a bridge mark IS (its anchors, query point, simulator, kernel,
// and coherence justification) so any consumer can read, filter, and audit one.
// It NEVER runs the simulator — the minting machinery lives under /internal. The
// package stays standard-library-only.
//
// The cardinal discipline: a bridge mark's provenance must make the pin/span
// boundary unmissable. The Category and TruthSource fields exist so a consumer
// can always filter the collection to identified-only marks and never see a
// simulated quantity laundered as ground truth.

// Category separates the two kinds of mark. `identified` marks are the current
// design-based (sharp/fuzzy RDD) marks — the pins. `bridge` marks are calibrated
// simulator estimates between identified anchors — the span. The categories are
// never pooled in scoring.
type Category string

const (
	// CategoryIdentified is a design-based mark (the default for existing marks).
	CategoryIdentified Category = "identified"
	// CategoryBridge is a simulator-bridged interpolation between identified anchors.
	CategoryBridge Category = "bridge"
)

// TruthSource is the hard provenance line. An identified mark's effect is real,
// design-based truth; a bridge mark's effect is a calibrated simulator estimate.
// It is immutable per category and never aggregated across categories.
type TruthSource string

const (
	// TruthIdentified marks a real, design-based effect.
	TruthIdentified TruthSource = "identified"
	// TruthSimulatorBridged marks a calibrated simulator-bridged estimate.
	TruthSimulatorBridged TruthSource = "simulator-bridged"
)

// BridgeSpec carries everything specific to a bridge mark. It is present iff the
// mark's Category is `bridge`. Every field is description the dossier renders and
// the validator checks; none of it executes a simulator.
type BridgeSpec struct {
	// Mechanism is the id of the underlying effect-curve mechanism all anchors
	// (and this bridge) lie on. Two anchors share a mechanism iff they share the
	// policy variable, the outcome construct, the population definition, and the
	// policy regime — the anchor-coherence requirement.
	Mechanism string `json:"mechanism"`

	// PolicyVariable names what x is on the mechanism's effect curve (e.g. cutoff
	// level, intensity, dose, time, rank).
	PolicyVariable string `json:"policy_variable"`

	// QueryPoint is the x this mark estimates τ at. It always lies strictly inside
	// the anchor hull (interpolation only — bracketing is enforced in Validate).
	QueryPoint float64 `json:"query_point"`

	// Anchors are the identified marks this bridge is pinned to (>=2). They must
	// bracket QueryPoint: at least one anchor strictly on each side.
	Anchors []AnchorRef `json:"anchors"`

	// Simulator records the stochadex model id+version+seed+input hashes that make
	// the bridge re-mintable byte-for-byte (same determinism rule as identified
	// marks).
	Simulator SimulatorRef `json:"simulator"`

	// Kernel is the GP discrepancy covariance family + hyperparameters — the
	// trust-decay assumption, shipped openly as the load-bearing provenance.
	Kernel KernelSpec `json:"discrepancy_kernel"`

	// AnchorCoherence is the structured justification that all anchors reflect the
	// same underlying mechanism. Mandatory; a bridge without it is rejected.
	AnchorCoherence AnchorCoherence `json:"anchor_coherence"`

	// Inference records HOW the honest interval was produced: which rung of the
	// inference ladder (closed-form / deterministic-moment / sampled) and the
	// tractability-gate verdict that certified it. It makes "determinism was
	// earned, not assumed" auditable. Present once the deterministic causal layer
	// mints the mark; nil on foundation-era bridges.
	Inference *InferenceRecord `json:"inference,omitempty"`
}

// InferenceRecord is the deterministic causal layer's provenance for a bridge
// mark's interval: the inference rung used and the tractability-gate verdict that
// licensed it. Pure description — schema never runs the gate.
type InferenceRecord struct {
	// Rung is the inference ladder rung that produced the interval:
	// `closed-form` (linear-Gaussian, exact), `deterministic-moment`
	// (mildly-nonlinear, moment-propagated), or `sampled` (gated, deferred — used
	// only when the mechanism left the deterministic regime).
	Rung string `json:"rung"`

	// Tractability is the Axis-B gate verdict: whether the mechanism stayed in the
	// deterministic regime and the statistics that decided it. A `sampled` rung is
	// only honest when this records a failed gate.
	Tractability *TractabilityVerdict `json:"tractability,omitempty"`
}

// TractabilityVerdict echoes the gate's measurement of whether a deterministic
// interval is honest for this mechanism: a non-Gaussian / strongly-nonlinear θ
// posterior fails the gate and is routed to the sampling path instead of being
// papered over with a tidy Gaussian interval. The statistics + tolerances ship so
// a reader can audit the boundary.
type TractabilityVerdict struct {
	Pass            bool    `json:"pass"`
	Linear          bool    `json:"linear"`           // mechanism detected linear-in-θ (exact regime)
	NonlinearityGap float64 `json:"nonlinearity_gap"` // unscented-vs-linearised predictive variance gap
	Skew            float64 `json:"skew"`             // propagated-prediction skew (non-Gaussianity proxy)
	LaplaceMisfit   float64 `json:"laplace_misfit"`   // NLP-vs-quadratic gap in nats (posterior non-Gaussianity)
	NonlinearityTol float64 `json:"nonlinearity_tol"`
	SkewTol         float64 `json:"skew_tol"`
	MisfitTol       float64 `json:"misfit_tol"`
	Reason          string  `json:"reason"`
}

// AnchorRef points at an identified mark used as a pin, recording the mark id and
// its position on the policy variable. Storing the policy point here lets the
// schema-level Validate enforce bracketing without loading the other marks.
type AnchorRef struct {
	MarkID      string  `json:"mark_id"`
	PolicyPoint float64 `json:"policy_point"`
}

// SimulatorRef identifies the stochadex forward model and its determinism record.
type SimulatorRef struct {
	ModelID     string            `json:"model_id"`
	Version     string            `json:"version"`
	Seed        *int64            `json:"seed,omitempty"`
	InputHashes map[string]string `json:"input_hashes,omitempty"`
}

// KernelSpec is the GP covariance family and its hyperparameters, plus the
// numerical jitter used when conditioning. This is the assumption a consumer sees
// the estimate rests on.
type KernelSpec struct {
	Family string             `json:"family"`
	Params map[string]float64 `json:"params,omitempty"`
	Jitter float64            `json:"jitter,omitempty"`
}

// AnchorCoherence is the bridge-specific load-bearing justification that the
// anchors lie on one mechanism. The flags are the structured claim; Justification
// is the prose a reader can disagree with. A bridge across anchors from different
// causal regimes is a category error, so this is mandatory.
type AnchorCoherence struct {
	SamePopulation       bool   `json:"same_population"`
	SameRegime           bool   `json:"same_regime"`
	SameOutcomeConstruct bool   `json:"same_outcome_construct"`
	Justification        string `json:"justification"`
}
