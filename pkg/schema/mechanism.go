package schema

import "fmt"

// A Mechanism is the entity that makes the collection a clean instance of the
// bridge-marks data model: a single policy mechanism with one effect-curve, on
// which identified marks sit as anchors and bridge marks interpolate. It is where
// anchor coherence is *defined*: two anchors share a mechanism iff they share the
// policy variable, the outcome construct, the population definition, and the
// regime. Grouping marks under a mechanism is what turns "several isolated marks"
// into an anchor family a bridge can span.
//
// Every mark carries a MechanismID. Today most mechanisms hold a single anchor
// (the repo's existing seams); the data model is identical whether a mechanism has
// one anchor or a whole family, which is exactly the point — nothing has to change
// structurally when a family arrives, only more anchors get the same MechanismID.
type Mechanism struct {
	// ID is the stable mechanism identifier marks reference via MechanismID.
	ID string `json:"id"`
	// Name is a short human label.
	Name string `json:"name"`
	// Domain is the broad area (Education, Health, Environment, …).
	Domain string `json:"domain"`
	// Description is prose a reader can disagree with.
	Description string `json:"description"`

	// The four coherence fields. Anchors are coherent (share this mechanism) iff
	// they agree on all four; a bridge across anchors that disagree is a category
	// error the validator rejects.
	PolicyVariable       string `json:"policy_variable"`       // what x is: cutoff level | intensity | dose | time | rank
	OutcomeConstruct     string `json:"outcome_construct"`     // the single outcome these anchors all measure
	PopulationDefinition string `json:"population_definition"` // who/what the units are
	Regime               string `json:"regime"`                // the policy/institutional regime they share
}

// canonicalMechanisms is the registry of known mechanisms. Each existing seam is a
// mechanism holding a single identified anchor today; the registry is the clean
// data-model instance the bridge-seams spec calls for, and the anchor family a
// bridge needs is created by adding more anchors under the SAME id (not by inventing
// a new structure). New seams add an entry here.
var canonicalMechanisms = map[string]Mechanism{
	"area-funding-eligibility": {
		ID:                   "area-funding-eligibility",
		Name:                 "Area-funding eligibility cutoff",
		Domain:               "Local government",
		Description:          "Area-based funding allocated by a deprivation/eligibility cutoff (e.g. UKSPF/IMD): crossing the threshold changes an area's eligibility. Anchors sit at eligibility cutoffs on the deprivation axis. (Declared seam; no admitted anchor yet — verified but too recent for an outcome.)",
		PolicyVariable:       "local-authority deprivation rank relative to the eligibility cutoff (rank)",
		OutcomeConstruct:     "the area's subsequent funded outcome",
		PopulationDefinition: "local authorities in the UK",
		Regime:               "deprivation/eligibility-cutoff area funding",
	},
	"floor-standards-p8": {
		ID:                   "floor-standards-p8",
		Name:                 "Progress 8 floor standard",
		Domain:               "Education",
		Description:          "The school-accountability floor on Progress 8: crossing below the floor triggers intervention/scrutiny. Anchors sit at floor cutoffs on the Progress 8 axis; bridges would span mid-range performance where no cutoff exists.",
		PolicyVariable:       "school Progress 8 score relative to the accountability floor (cutoff level)",
		OutcomeConstruct:     "the school's subsequent-year Progress 8",
		PopulationDefinition: "state-funded mainstream secondary schools in England",
		Regime:               "Progress 8 floor-standards accountability (2016–)",
	},
	"shmi-mortality-banding": {
		ID:                   "shmi-mortality-banding",
		Name:                 "SHMI mortality banding",
		Domain:               "Health",
		Description:          "The Summary Hospital-level Mortality Indicator banding: crossing the higher-than-expected band boundary changes scrutiny/recommended action (fuzzy). Anchors sit at band boundaries on the SHMI axis.",
		PolicyVariable:       "trust SHMI relative to the higher-than-expected band boundary (cutoff level)",
		OutcomeConstruct:     "the trust's subsequent SHMI (relative mortality)",
		PopulationDefinition: "NHS acute (non-specialist) trusts in England",
		Regime:               "SHMI banding (fuzzy assignment)",
	},
	"bathing-water-classification": {
		ID:                   "bathing-water-classification",
		Name:                 "Bathing-water classification compliance",
		Domain:               "Environment",
		Description:          "The revised Bathing Water Directive classification: crossing a band boundary on the compliance margin changes the classification and its consequences (advice-against-bathing, investigation). The band boundaries (Poor/Sufficient, Sufficient/Good, Good/Excellent) are anchors on ONE log-compliance-margin axis — the natural first anchor family.",
		PolicyVariable:       "log compliance margin over a classification band threshold (cutoff level)",
		OutcomeConstruct:     "the site's subsequent-window log compliance margin",
		PopulationDefinition: "designated bathing waters in England",
		Regime:               "rBWD (2015–) classification",
	},
}

// MechanismByID returns the canonical mechanism for an id, or false if unknown.
func MechanismByID(id string) (Mechanism, bool) {
	m, ok := canonicalMechanisms[id]
	return m, ok
}

// CanonicalMechanisms returns a copy of the mechanism registry, so consumers can
// read the data model's mechanism entities alongside the marks.
func CanonicalMechanisms() map[string]Mechanism {
	out := make(map[string]Mechanism, len(canonicalMechanisms))
	for k, v := range canonicalMechanisms {
		out[k] = v
	}
	return out
}

// validateMechanismID checks a mark's MechanismID is present and known, and — for
// bridge marks — that it agrees with the bridge block's mechanism (the two must name
// the same mechanism, or the pin/span provenance is incoherent).
func (m Mark) validateMechanismID() error {
	if m.MechanismID == "" {
		return fmt.Errorf("mark %q: missing mechanism_id (every mark belongs to a mechanism)", m.ID)
	}
	if _, ok := MechanismByID(m.MechanismID); !ok {
		return fmt.Errorf("mark %q: unknown mechanism_id %q (add it to the mechanism registry)", m.ID, m.MechanismID)
	}
	if m.EffectiveCategory() == CategoryBridge && m.Bridge != nil && m.Bridge.Mechanism != m.MechanismID {
		return fmt.Errorf("mark %q: bridge mechanism %q must equal mechanism_id %q", m.ID, m.Bridge.Mechanism, m.MechanismID)
	}
	return nil
}
