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
	"emission-zone-stringency-to-roadside-no2": {
		ID:                   "emission-zone-stringency-to-roadside-no2",
		Name:                 "Emission-zone stringency → roadside NO2",
		Domain:               "Environment",
		Description:          "Low-emission-zone stringency and its effect on roadside NO2: switching on an emission zone restricts non-compliant vehicles and should cut kerbside NO2. The London ULEZ expansions (central 2019, inner 2021, London-wide 2023) and European city LEZs would be anchors at different stringency/coverage points on ONE policy axis. DECLARED SEAM, NO ADMITTED ANCHOR YET: the controlled-ITS build (internal/series/ulezno2.go) was implemented and run on real LAQN data for both the 2019 and 2023 London events, but neither cleanly identifies — COVID contaminates the entire usable window (the 2019 post-period has only ~10 months before the Mar-2020 traffic collapse; the 2023 pre-period IS the post-COVID rebound, whose nonlinear curvature the placebo battery correctly flags). The plug-in effect is negative (~-5 to -6 ug/m3 for 2019, consistent with the literature) but is not robust to the counterfactual trend specification, so the honest model-averaged interval spans zero and the marks are reported-but-not-admitted. A pre-COVID European LEZ (Berlin Umweltzone, Milan Area C) would be the cleaner first anchor; see research/2026-06-04-ulez-no2-its-covid-confound.md.",
		PolicyVariable:       "emission-zone stringency/coverage at a sharp switch-on instant (time / dose)",
		OutcomeConstruct:     "roadside NO2 concentration relative to an urban-background control series",
		PopulationDefinition: "roadside air-quality monitoring stations within the zone's airshed",
		Regime:               "standard-based low-emission zone (daily charge for non-compliant vehicles)",
	},
	"lez-ban-stringency-to-roadside-no2": {
		ID:                   "lez-ban-stringency-to-roadside-no2",
		Name:                 "LEZ ban stringency → roadside NO2",
		Domain:               "Environment",
		Description:          "Standard-BAN low-emission zones (non-compliant vehicles prohibited, not charged) and their effect on roadside NO2 — kept SEPARATE from charge-type zones (London ULEZ) so a future bridge interpolates within a coherent mechanism. First anchor: Berlin Umweltzone stage 2 (2010, green-sticker Euro-4-diesel ban inside the S-Bahn Ring). NOTE: LEZ effects on NO2 are known to be modest (the Euro-standard upgrades cut particulates/soot far more than NOx, since real-world diesel NOx stayed high through Euro 4/5), so anchors on this mechanism may carry near-null central effects with honest wide intervals — a valid causal result.",
		PolicyVariable:       "emission-zone ban stringency at a sharp switch-on instant (time / dose)",
		OutcomeConstruct:     "roadside NO2 concentration relative to an urban-background control series",
		PopulationDefinition: "roadside air-quality monitoring stations within the zone's airshed",
		Regime:               "standard-based low-emission zone (ban on non-compliant vehicles)",
	},
	"menthol-restriction-to-smoking": {
		ID:                   "menthol-restriction-to-smoking",
		Name:                 "Menthol/flavour restriction → smoking prevalence",
		Domain:               "Health",
		Description:          "Bans on menthol (and other characterising flavours) in cigarettes and their effect on adult smoking prevalence, identified by difference-in-differences across jurisdictions that restrict at different times/intensities. First anchor: Canada's staggered provincial menthol bans (2015-2017), treated vs the provinces covered only by the federal Oct-2017 ban. The intended anchor family spans a restriction-INTENSITY axis — menthol-only (Canadian provinces) through comprehensive flavour bans (Massachusetts 2020, California 2022) — across two countries, the cross-country bridge target. Effects on TOTAL smoking are diluted by substitution to non-menthol products, so central effects are smaller than the menthol-specific reductions in the literature.",
		PolicyVariable:       "flavour-restriction intensity at a jurisdiction's ban effective date (dose / time)",
		OutcomeConstruct:     "adult current-smoking prevalence relative to a not-yet-restricting control group",
		PopulationDefinition: "the adult population of a province/state with a survey-measured smoking rate",
		Regime:               "characterising-flavour (menthol) sales restriction",
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
