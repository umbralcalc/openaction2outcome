// Package schema defines the public, dependency-light data types for the
// openaction2outcome causal yardstick: the Mark (a causally-identified
// reference point carrying honest identification uncertainty) and the
// Submission (a model-under-test's predicted effects).
//
// This package deliberately imports nothing beyond the standard library so
// that consumers who only want to read marks or build submissions pull a tiny
// dependency tree. The heavy minting machinery (RDD estimation, SBI, the
// validity battery) lives under /internal and never appears in this import
// graph.
package schema

// SchemaVersion is the version of the Mark/Submission schema. It is written
// into every emitted Mark and is checked by the evaluator. Bump on any
// breaking change to the wire format.
const SchemaVersion = "0.5.0"

// Series identifies which institutional-decision family a mark belongs to.
type Series string

const (
	SeriesAreaFunding    Series = "area-funding"    // local authority × deprivation/eligibility cutoff
	SeriesFloorStandards Series = "floor-standards" // school × performance floor
	SeriesSHMI           Series = "shmi"            // NHS trust × mortality banding (fuzzy)
	SeriesBathingWater   Series = "bathing-water"   // designated bathing water × Poor/Sufficient classification (sharp)
	SeriesULEZNO2        Series = "ulez-no2"        // London ULEZ expansion × roadside NO2 (controlled ITS)
	SeriesBerlinLEZ      Series = "berlin-lez-no2"  // Berlin Umweltzone × roadside NO2 (controlled ITS)
	SeriesMadridLEZ      Series = "madrid-lez-no2"  // Madrid Central LEZ × in-zone NO2 (controlled ITS)
	SeriesCAMenthol      Series = "ca-menthol-smoking" // Canadian provincial menthol bans × smoking (DiD)
)

// RDDType distinguishes sharp assignment (crossing the cutoff deterministically
// assigns the action) from fuzzy assignment (crossing the cutoff changes the
// probability of the action).
type RDDType string

const (
	Sharp RDDType = "sharp"
	Fuzzy RDDType = "fuzzy"
	// Kink is a regression-kink design: assignment intensity is a continuous,
	// deterministic function of the running variable whose SLOPE changes at the
	// cutoff (a tiered relief/benefit taper). The effect is identified from the
	// kink in the outcome's slope, not a level jump. See Design.PolicySlopeChange.
	Kink RDDType = "kink"
	// DiD is a difference-in-differences design: the effect is identified by
	// comparing a treated group's pre→post change to a control group's, under a
	// parallel-trends assumption. It has no cutoff/running-variable discontinuity;
	// it is the anchor unit for dose / staggered-rollout mechanisms (a policy
	// intensity ratcheted in steps, or a scheme rolled out across areas/times).
	DiD RDDType = "did"
)

// Identification is the design-family discriminator that selects which `design`
// sub-shape and which `dossier` block a reader should expect. It is the forward
// generalisation of `rdd_type`: a mark is identification-agnostic everywhere
// except those two polymorphic blocks. Old marks that carry only `rdd_type`
// migrate via EffectiveIdentification; new marks (notably ITS) set this field
// directly and may leave `rdd_type` empty.
type Identification string

const (
	IDRDDSharp Identification = "rdd-sharp" // sharp regression discontinuity
	IDRDDFuzzy Identification = "rdd-fuzzy" // fuzzy regression discontinuity
	IDRDDKink  Identification = "rdd-kink"  // regression-kink design
	IDDiD      Identification = "did"       // difference-in-differences

	// IDITSControlled is a controlled interrupted time series: the effect is a
	// POPULATION effect accumulated over a post-intervention window, identified by
	// comparing a treated series' break at a sharp intervention instant to a
	// control series under a parallel-trends-in-time assumption. Its estimand is
	// not local-at-cutoff, so its decision scores never pool with RDD marks.
	IDITSControlled Identification = "its-controlled"
)

// RowShape declares the shape of a mark's episode rows. RDD/DiD marks are a
// cross-section (one row per unit × decision-period); ITS marks are a panel (one
// row per series × time bucket). A reader keys its row decoder off it.
type RowShape string

const (
	RowCrossSection RowShape = "cross-section"
	RowPanel        RowShape = "panel"
)

// rddToIdentification maps the legacy rdd_type discriminator onto the forward
// identification discriminator. An unknown/empty rdd_type maps to "".
func rddToIdentification(t RDDType) Identification {
	switch t {
	case Sharp:
		return IDRDDSharp
	case Fuzzy:
		return IDRDDFuzzy
	case Kink:
		return IDRDDKink
	case DiD:
		return IDDiD
	default:
		return ""
	}
}
