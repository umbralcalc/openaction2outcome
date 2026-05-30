package sbi

import (
	"math"

	"github.com/umbralcalc/openaction2outcome/internal/rdd"
)

// This file implements the fuzzy regression-discontinuity estimator. When
// crossing the cutoff changes only the PROBABILITY of treatment (not treatment
// itself), the effect is the Wald ratio of two discontinuities at the cutoff:
//
//	LATE = (jump in the outcome) / (jump in the treatment probability)
//	     = reduced-form jump (rho) / first-stage jump (pi)
//
// Both jumps are estimated per specification with the same model-averaged SMC
// machinery as the sharp case; the LATE for a spec is rho/pi propagated by the
// delta method, which keeps the per-spec estimate Gaussian so the whole thing
// reuses the model-averaging mixture. A fuzzy mark is only trustworthy when the
// first stage is strong, so the estimator also reports the aggregated first-stage
// strength for the admission gate.

// FuzzyPoint is one unit: running variable X, realized treatment D in {0,1}
// (whether the unit actually received the action), and outcome Y.
type FuzzyPoint struct {
	X float64
	D float64
	Y float64
}

// FirstStageSummary is the model-averaged jump in treatment probability at the
// cutoff and its strength.
type FirstStageSummary struct {
	Jump   float64 // weighted first-stage jump (pi)
	SD     float64
	FStat  float64 // (Jump/SD)^2 — a weak-instrument diagnostic
	Passed bool    // FStat >= the weak-instrument threshold
}

// FuzzyResult is the fuzzy estimate: the LATE posterior, the first-stage summary
// that gates admission, and the (model-averaged) reduced-form jump for context.
type FuzzyResult struct {
	LATE               BMAResult
	FirstStage         FirstStageSummary
	ReducedFormCentral float64
}

// fuzzyFStatThreshold is the conventional first-stage F-statistic above which the
// instrument is considered strong enough to trust the Wald ratio.
const fuzzyFStatThreshold = 10.0

// EstimateFuzzyBMA estimates the fuzzy LATE. treatedBelow selects which side of
// the cutoff is encouraged into treatment.
func EstimateFuzzyBMA(pts []FuzzyPoint, cutoff float64, specs []Spec, treatedBelow bool, cfg SMCConfig) FuzzyResult {
	rf := make([]rdd.Point, len(pts)) // reduced form: outcome on running
	fs := make([]rdd.Point, len(pts)) // first stage: treatment on running
	for i, p := range pts {
		rf[i] = rdd.Point{X: p.X, Y: p.Y}
		fs[i] = rdd.Point{X: p.X, Y: p.D}
	}

	type stageRec struct{ mean, variance float64 }
	piBySpec := make(map[Spec]stageRec)
	rhoBySpec := make(map[Spec]stageRec)
	var ests []specEstimate
	for _, s := range specs {
		rfFit := buildSpecFit(rf, cutoff, s, treatedBelow)
		fsFit := buildSpecFit(fs, cutoff, s, treatedBelow)
		pRF := fitSpecSMC(rfFit, cfg)
		pFS := fitSpecSMC(fsFit, cfg)
		if !pRF.ok || !pFS.ok || math.IsNaN(pRF.logZ) || math.IsInf(pRF.logZ, 0) {
			continue
		}
		rho, rhoVar := pRF.tauMean, pRF.tauVar
		pi, piVar := pFS.tauMean, pFS.tauVar
		if math.Abs(pi) < 1e-6 {
			continue // ratio undefined at this spec
		}
		// Delta method for LATE = rho/pi with independent stages.
		lateMean := rho / pi
		lateVar := rhoVar/(pi*pi) + (rho*rho)/(pi*pi*pi*pi)*piVar
		if lateVar <= 0 || math.IsNaN(lateVar) || math.IsInf(lateVar, 0) {
			continue
		}
		ests = append(ests, specEstimate{spec: s, mean: lateMean, variance: lateVar, logZ: pRF.logZ})
		piBySpec[s] = stageRec{mean: pi, variance: piVar}
		rhoBySpec[s] = stageRec{mean: rho, variance: rhoVar}
	}

	late := assembleBMA(ests)

	// Aggregate the first stage (and reduced form) using the same model-average
	// weights, with the law of total variance for the first-stage SD.
	var piMean, rhoMean float64
	for _, sw := range late.Specs {
		piMean += sw.Weight * piBySpec[sw.Spec].mean
		rhoMean += sw.Weight * rhoBySpec[sw.Spec].mean
	}
	var piVar float64
	for _, sw := range late.Specs {
		r := piBySpec[sw.Spec]
		d := r.mean - piMean
		piVar += sw.Weight * (r.variance + d*d)
	}
	piSD := math.Sqrt(piVar)
	fstat := 0.0
	if piSD > 0 {
		fstat = (piMean / piSD) * (piMean / piSD)
	}
	return FuzzyResult{
		LATE: late,
		FirstStage: FirstStageSummary{
			Jump: piMean, SD: piSD, FStat: fstat, Passed: fstat >= fuzzyFStatThreshold,
		},
		ReducedFormCentral: rhoMean,
	}
}
