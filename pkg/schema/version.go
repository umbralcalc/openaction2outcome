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
const SchemaVersion = "0.2.0"

// Seam identifies which institutional-decision family a mark belongs to.
type Seam string

const (
	SeamAreaFunding    Seam = "area-funding"    // local authority × deprivation/eligibility cutoff
	SeamFloorStandards Seam = "floor-standards" // school × performance floor
	SeamSHMI           Seam = "shmi"            // NHS trust × mortality banding (fuzzy)
)

// RDDType distinguishes sharp assignment (crossing the cutoff deterministically
// assigns the action) from fuzzy assignment (crossing the cutoff changes the
// probability of the action).
type RDDType string

const (
	Sharp RDDType = "sharp"
	Fuzzy RDDType = "fuzzy"
)
