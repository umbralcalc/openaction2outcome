package bridge

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// This file is the deterministic causal layer's reference mechanism: a directed
// structural causal model built as a stochadex deterministic graph, with analytic
// `do(·)` interventions. It is the concrete demonstration that a *causal* (Axis-A)
// mechanism can be fully closed-form / linear-Gaussian (Axis-B) — the whole point
// of the determinism-first doctrine: "this mechanism is causal" does NOT force
// sampling; only intractability does.
//
// The graph (a confounded treatment→outcome DAG):
//
//	      C  (confounder, exogenous)
//	     ╱ ╲
//	    ▼   ▼
//	    T ─▶ Y     T = treatment (the policy lever), Y = outcome
//
// Structural equations (linear-Gaussian, means propagated deterministically):
//
//	C = c0
//	T = aT + gCT·C                     (observational regime)
//	Y = αY + β·T + gCY·C
//
// The policy variable x is the treatment level, and the bridge estimates the
// INTERVENTIONAL effect curve τ(x) = E[Y | do(T = x)]. The intervention is
// analytic: do(T=x) severs the C→T edge and pins T to x, so
//
//	τ(x; θ) = αY + gCY·c0 + β·x .
//
// This differs from the OBSERVATIONAL association of Y on T, whose slope is
// confounded by the open back-door path T←C→Y (β + gCY·gCT·VarC / VarT). A plain
// covariance/kernel could only ever recover the observational conditional; the
// directed graph is what answers do(·). The contrast is exposed by
// ObservationalSlope / InterventionalSlope and checked in the tests.
//
// Each node is a stochadex `ValuesFunctionIteration` and each arrow is a
// `ParamsFromUpstream` edge, so the mechanism is a genuine stochadex graph run on
// the same simulator engine the identified marks use — deterministic and
// re-mintable byte-for-byte. Because it is linear in θ, the deterministic moment
// calibrator reproduces the analytic posterior over τ(query) exactly.

// LinearSCMMechanism is the directed linear-Gaussian structural causal model above.
// The inferred parameters are θ = [β, αY] (the causal slope and the outcome
// intercept); the remaining structural constants are fixed, known model structure
// that the intervention propagates through the graph.
type LinearSCMMechanism struct {
	C0      float64 // confounder mean E[C]
	GammaCY float64 // C → Y structural coefficient (confounding into the outcome)
	GammaCT float64 // C → T structural coefficient (confounding into the treatment)
	AT      float64 // treatment intercept (observational regime only)
	VarC    float64 // Var(C), used only for the observational-slope contrast
	VarT    float64 // residual Var(T) given C, used only for the observational-slope contrast
	version string
}

// NewLinearSCMMechanism returns the reference confounded treatment→outcome SCM with
// defensible default structure (positive confounding on both arms).
func NewLinearSCMMechanism() LinearSCMMechanism {
	return LinearSCMMechanism{
		C0: 1.0, GammaCY: 0.8, GammaCT: 0.6, AT: 0.0, VarC: 1.0, VarT: 0.5,
		version: "linear-scm-1",
	}
}

// Predict returns m(x; θ) = E[Y | do(T = x)] computed by running the interventional
// stochadex graph (C→T edge severed, T pinned to x) to termination and reading the
// outcome node. β = θ[0], αY = θ[1].
func (m LinearSCMMechanism) Predict(x float64, theta []float64) float64 {
	return m.runGraph(true, x, theta[0], theta[1])
}

func (m LinearSCMMechanism) ParamDim() int { return 2 }

func (m LinearSCMMechanism) Priors() ([]inference.Prior, []string) {
	return []inference.Prior{
		&inference.UniformPrior{Lo: -10, Hi: 10}, // β: causal slope
		&inference.UniformPrior{Lo: -10, Hi: 10}, // αY: outcome intercept
	}, []string{"beta", "alpha_y"}
}

func (m LinearSCMMechanism) ID() string      { return "linear-scm-causal" }
func (m LinearSCMMechanism) Version() string { return m.version }

// InterventionalSlope is dτ/dx under do(T=x): the pure causal effect β.
func (m LinearSCMMechanism) InterventionalSlope(theta []float64) float64 { return theta[0] }

// ObservationalSlope is the slope a naive regression of Y on T would recover. The
// open back-door T←C→Y biases it by gCY·gCT·VarC / VarT, so it is NOT the causal
// effect — the whole reason the directed graph is required. Exposed for the
// dossier/test contrast, not used in calibration.
func (m LinearSCMMechanism) ObservationalSlope(theta []float64) float64 {
	beta := theta[0]
	varTtotal := m.GammaCT*m.GammaCT*m.VarC + m.VarT
	if varTtotal <= 0 {
		return beta
	}
	return beta + m.GammaCY*m.GammaCT*m.VarC/varTtotal
}

// runGraph builds and runs the deterministic stochadex graph and returns the final
// outcome-node value. With intervene=true the C→T edge is severed and T is pinned
// to x (the do-operator); with intervene=false T follows its structural equation
// (the observational regime). The graph is run for a few steps so the affine chain
// C→T→Y fully propagates; with constant exogenous inputs it reaches the exact
// composed value and holds it.
func (m LinearSCMMechanism) runGraph(intervene bool, x, beta, alphaY float64) float64 {
	const (
		cIdx = 0
		tIdx = 1
		yIdx = 2
		// Steps needed for the signal to propagate the full depth of the DAG
		// (C→T→Y is depth two); a few extra steps are harmless since the inputs
		// are constant and the map is affine.
		steps = 4
	)

	c0, gammaCY, gammaCT, aT := m.C0, m.GammaCY, m.GammaCT, m.AT

	// Node C: the exogenous confounder, constant at its mean.
	cNode := &general.ValuesFunctionIteration{Function: func(
		_ *simulator.Params, _ int, _ []*simulator.StateHistory, _ *simulator.CumulativeTimestepsHistory,
	) []float64 {
		return []float64{c0}
	}}

	// Node T: the treatment. Under do(T=x) it is pinned to x (parents severed);
	// otherwise it follows T = aT + gCT·C, reading C from the upstream edge.
	tNode := &general.ValuesFunctionIteration{Function: func(
		params *simulator.Params, _ int, _ []*simulator.StateHistory, _ *simulator.CumulativeTimestepsHistory,
	) []float64 {
		if intervene {
			return []float64{x}
		}
		return []float64{aT + gammaCT*params.GetIndex("c", 0)}
	}}

	// Node Y: the outcome, Y = αY + β·T + gCY·C, reading both upstream edges.
	yNode := &general.ValuesFunctionIteration{Function: func(
		params *simulator.Params, _ int, _ []*simulator.StateHistory, _ *simulator.CumulativeTimestepsHistory,
	) []float64 {
		return []float64{alphaY + beta*params.GetIndex("t", 0) + gammaCY*params.GetIndex("c", 0)}
	}}

	settings := &simulator.Settings{
		InitTimeValue:         0,
		TimestepsHistoryDepth: 1,
		Iterations: []simulator.IterationSettings{
			{Name: "C", Params: simulator.NewParams(nil), InitStateValues: []float64{c0}, StateWidth: 1, StateHistoryDepth: 1},
			{
				Name:               "T",
				Params:             simulator.NewParams(nil),
				ParamsFromUpstream: map[string]simulator.UpstreamConfig{"c": {Upstream: cIdx, Indices: []int{0}}},
				InitStateValues:    []float64{0},
				StateWidth:         1,
				StateHistoryDepth:  1,
			},
			{
				Name:   "Y",
				Params: simulator.NewParams(nil),
				ParamsFromUpstream: map[string]simulator.UpstreamConfig{
					"t": {Upstream: tIdx, Indices: []int{0}},
					"c": {Upstream: cIdx, Indices: []int{0}},
				},
				InitStateValues:   []float64{0},
				StateWidth:        1,
				StateHistoryDepth: 1,
			},
		},
	}
	settings.Init()

	store := simulator.NewStateTimeStorage()
	impls := &simulator.Implementations{
		Iterations:           []simulator.Iteration{cNode, tNode, yNode},
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1},
	}
	for i, it := range impls.Iterations {
		it.Configure(i, settings)
	}
	coord := simulator.NewPartitionCoordinator(settings, impls)
	coord.Run()

	rows := store.GetValues("Y")
	if len(rows) == 0 || len(rows[len(rows)-1]) == 0 {
		return alphaY + beta*x + gammaCY*c0 // defensive analytic fallback
	}
	return rows[len(rows)-1][0]
}
