// Package hfexport turns the minted marks into a Hugging Face-loadable dataset:
// one flattened JSON record per mark, grouped into a file per series, plus the
// Dataset Card. The flattened record carries everything a model evaluator needs
// — the decision context, the effect distribution (central + interval +
// quantiles + samples), and a link to the full episode table — while the
// complete nested mark stays in the repo (referenced by mark_json_path).
package hfexport

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// Record is the flat, columnar view of a mark for Hugging Face.
type Record struct {
	ID       string `json:"id"`
	Series   string `json:"series"`
	Domain   string `json:"domain"`
	UnitType string `json:"unit_type"`
	RDDType  string `json:"rdd_type"`

	RunningVariable     string  `json:"running_variable"`
	RunningVariableDesc string  `json:"running_variable_description"`
	Cutoff              float64 `json:"cutoff"`
	Direction           string  `json:"direction"`
	Action              string  `json:"action"`
	Alternative         string  `json:"alternative"`
	Outcome             string  `json:"outcome"`
	OutcomeDesc         string  `json:"outcome_description"`
	Estimand            string  `json:"estimand"`

	ContextDescription string   `json:"context_description"`
	Population         string   `json:"population"`
	CovariateNames     []string `json:"covariate_names"`

	EffectCentral float64    `json:"effect_central"`
	EffectLevel   float64    `json:"effect_interval_level"`
	EffectLower   float64    `json:"effect_lower"`
	EffectUpper   float64    `json:"effect_upper"`
	EffectStdDev  float64    `json:"effect_std_dev"`
	SamplingSD    float64    `json:"effect_sampling_sd"`
	IdentSD       float64    `json:"effect_identification_sd"`
	Quantiles     []quantile `json:"effect_quantiles"`
	Samples       []float64  `json:"effect_samples"`

	Admitted bool `json:"admitted"`

	EpisodeURL     string   `json:"episode_table_url"`
	EpisodeSHA256  string   `json:"episode_table_sha256"`
	EpisodeRows    int      `json:"episode_table_rows"`
	EpisodeColumns []string `json:"episode_table_columns"`

	ContextAsOf       string   `json:"context_asof"`
	DecisionTimestamp string   `json:"decision_timestamp"`
	OutcomeTimestamp  string   `json:"outcome_timestamp"`
	OutcomeRealized   bool     `json:"outcome_realized"`
	Sources           []source `json:"sources"`

	MarkJSONPath string `json:"mark_json_path"`
}

type quantile struct {
	P     float64 `json:"p"`
	Value float64 `json:"value"`
}

type source struct {
	Title     string `json:"title"`
	Publisher string `json:"publisher"`
	Licence   string `json:"licence"`
}

// ToRecord flattens a mark.
func ToRecord(m schema.Mark) Record {
	r := Record{
		ID: m.ID, Series: string(m.Series), Domain: m.Domain, UnitType: m.UnitType, RDDType: string(m.RDDType),
		RunningVariable: m.Design.RunningVariable.Name, RunningVariableDesc: m.Design.RunningVariable.Description,
		Cutoff: m.Design.Cutoff, Direction: string(m.Design.Direction),
		Action: m.Design.Action, Alternative: m.Design.Alternative,
		Outcome: m.Design.Outcome.Name, OutcomeDesc: m.Design.Outcome.Description, Estimand: m.Design.Estimand,
		ContextDescription: m.Context.Description, Population: m.Context.Population, CovariateNames: m.Context.CovariateNames,
		EffectCentral: m.Effect.Central, Samples: m.Effect.Samples,
		Admitted:   m.Dossier.Admitted,
		EpisodeURL: m.Data.URI, EpisodeSHA256: m.Data.SHA256, EpisodeRows: m.Data.Rows, EpisodeColumns: m.Data.Columns,
		ContextAsOf: m.Provenance.ContextAsOf, DecisionTimestamp: m.Provenance.DecisionTimestamp,
		OutcomeTimestamp: m.Provenance.OutcomeTimestamp, OutcomeRealized: m.Provenance.OutcomeRealized,
		MarkJSONPath: "marks/" + m.ID + ".json",
	}
	if m.Effect.Interval != nil {
		r.EffectLevel, r.EffectLower, r.EffectUpper = m.Effect.Interval.Level, m.Effect.Interval.Lower, m.Effect.Interval.Upper
	}
	if m.Effect.StdDev != nil {
		r.EffectStdDev = *m.Effect.StdDev
	}
	if ub := m.Effect.UncertaintyBudget; ub != nil {
		if ub.Sampling != nil {
			r.SamplingSD = math.Sqrt(*ub.Sampling)
		}
		if ub.Specification != nil {
			r.IdentSD = math.Sqrt(*ub.Specification)
		}
	}
	for _, q := range m.Effect.Quantiles {
		r.Quantiles = append(r.Quantiles, quantile{P: q.P, Value: q.Value})
	}
	for _, s := range m.Provenance.Sources {
		r.Sources = append(r.Sources, source{Title: s.Title, Publisher: s.Publisher, Licence: s.Licence})
	}
	return r
}

// Export writes a Hugging Face-ready directory under outDir: one <series>.jsonl
// per series (records sorted by id for determinism) and the Dataset Card as
// README.md. cardPath is the committed Dataset Card to copy.
func Export(marks []schema.Mark, outDir, cardPath string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	bySeries := make(map[string][]Record)
	for _, m := range marks {
		r := ToRecord(m)
		bySeries[r.Series] = append(bySeries[r.Series], r)
	}
	series := make([]string, 0, len(bySeries))
	for s := range bySeries {
		series = append(series, s)
	}
	sort.Strings(series)
	for _, s := range series {
		recs := bySeries[s]
		sort.Slice(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
		// Hugging Face split names must be [a-z0-9_]; the file is named to match.
		fname := strings.ReplaceAll(s, "-", "_") + ".jsonl"
		f, err := os.Create(filepath.Join(outDir, fname))
		if err != nil {
			return err
		}
		enc := json.NewEncoder(f)
		for _, r := range recs {
			if err := enc.Encode(r); err != nil {
				f.Close()
				return err
			}
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	card, err := os.ReadFile(cardPath)
	if err != nil {
		return fmt.Errorf("read dataset card %s: %w", cardPath, err)
	}
	return os.WriteFile(filepath.Join(outDir, "README.md"), card, 0o644)
}
