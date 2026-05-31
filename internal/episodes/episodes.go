// Package episodes reshapes every mark's per-unit rows into the single published
// `episodes` dataset: one row per unit, across all series, in the terms the marks
// already use — the unit's context before the decision (its covariates and where
// it sat relative to the cutoff), what was done to it (assigned / treated), and
// the outcome that followed. That is the (state, action, reward) view a model
// trainer wants, named the way the rest of the project names things.
//
// It is a pure reshape: it reads the episode rows a build staged under dist/ (a
// build intermediate), joins each row with its mark's design and a small scalar
// effect summary, and writes one deterministic Parquet plus a slim git-tracked
// manifest pointing at the object-storage copy.
//
// Only two artifacts are ever stored: the marks (metadata, in git) and this
// episodes dataset (rows, in object storage). They join on the mark id — the rows
// carry mark_id so the per-mark metadata is normalised out, not duplicated. The
// full effect distribution stays in the mark; each row carries only mark_id (the
// join key) plus a few scalar effect summaries for convenience.
package episodes

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/snappy"

	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// stagedTableName is the fixed filename a build stages each mark's rows under
// (dist/marks/<mark_id>/<stagedTableName>).
const stagedTableName = "episodes.csv.gz"

// coreColumns are the fixed leading columns every series' episode table carries.
// Everything after them is a series-specific pre-treatment covariate that flows
// into the row's covariates list.
var coreColumns = map[string]bool{
	"unit_id": true, "unit_name": true, "running_value": true,
	"assigned": true, "treated": true, "outcome": true,
}

// Covariate is one pre-treatment covariate of a unit's context. Covariates are
// carried as a key-sorted list of these (rather than a Go map) so the Parquet
// bytes are deterministic: Go map iteration order is randomised, which would
// break content-addressing.
type Covariate struct {
	Name  string  `parquet:"name"`
	Value float64 `parquet:"value"`
}

// Row is one unit's episode: its context before the decision (state), what was
// done (action), and the outcome that followed (reward). It unions every series;
// series-specific covariates live in Covariates.
type Row struct {
	MarkID string `parquet:"mark_id"`
	Series string `parquet:"series"`

	UnitID   string `parquet:"unit_id"`
	UnitName string `parquet:"unit_name"`

	// Context before the decision (the "state"): where the unit sat relative to
	// the cutoff, plus its pre-treatment covariates.
	RunningValue     float64     `parquet:"running_value"`
	Cutoff           float64     `parquet:"cutoff"`
	DistanceToCutoff float64     `parquet:"distance_to_cutoff"`
	Direction        string      `parquet:"direction"`
	Covariates       []Covariate `parquet:"covariates"`

	// What was done (the "action"): the assigned side, the realized receipt
	// (nullable under fuzzy assignment), and the textual action / counterfactual.
	Assigned    bool   `parquet:"assigned"`
	Treated     *bool  `parquet:"treated,optional"`
	Action      string `parquet:"action"`
	Alternative string `parquet:"alternative"`

	// What followed (the "reward"): the later observed outcome. OutcomeObserved
	// is false (and Outcome nil) when the unit has no linked outcome (e.g.
	// attrition).
	Outcome         *float64 `parquet:"outcome,optional"`
	OutcomeObserved bool     `parquet:"outcome_observed"`

	// Inlined scalar summary of the mark's effect (convenience only; the full
	// posterior stays in the mark, joinable on mark_id).
	EffectCentral       float64 `parquet:"effect_central"`
	EffectLower         float64 `parquet:"effect_lower"`
	EffectUpper         float64 `parquet:"effect_upper"`
	EffectIntervalLevel float64 `parquet:"effect_interval_level"`
	EffectStdDev        float64 `parquet:"effect_std_dev"`
}

// Columns is the column order of the Row Parquet schema, recorded in the manifest
// so consumers can see the shape without reading the file. The covariates are
// nested under "covariates" (a list of {name,value}).
var Columns = []string{
	"mark_id", "series", "unit_id", "unit_name",
	"running_value", "cutoff", "distance_to_cutoff", "direction", "covariates",
	"assigned", "treated", "action", "alternative",
	"outcome", "outcome_observed",
	"effect_central", "effect_lower", "effect_upper",
	"effect_interval_level", "effect_std_dev",
}

// LoadTable reads the episode rows a build staged for the given mark under
// distDir/marks/<id>/episodes.csv.gz and returns the parsed header and rows. It
// is offline-first: the rows must already be staged (run `openaction2outcome
// build` first). The staged file is a deterministic build intermediate, so it is
// trusted by convention; the published artifacts (the marks in git, the episodes
// Parquet by its manifest SHA) carry the integrity guarantees.
func LoadTable(m schema.Mark, distDir string) (header []string, rows [][]string, err error) {
	p := filepath.Join(distDir, "marks", m.ID, stagedTableName)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("episode rows not staged for mark %q at %s (run `openaction2outcome build`)", m.ID, p)
		}
		return nil, nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, nil, fmt.Errorf("mark %q episode rows: %w", m.ID, err)
	}
	defer gz.Close()
	all, err := csv.NewReader(gz).ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("mark %q episode rows: %w", m.ID, err)
	}
	if len(all) == 0 {
		return nil, nil, fmt.Errorf("mark %q episode rows are empty", m.ID)
	}
	return all[0], all[1:], nil
}

// Build reshapes every mark's staged episode rows into the unified rows, sorted
// deterministically by (series, mark_id, unit_id).
func Build(marks []schema.Mark, distDir string) ([]Row, error) {
	var out []Row
	for _, m := range marks {
		header, rows, err := LoadTable(m, distDir)
		if err != nil {
			return nil, err
		}
		idx := columnIndex(header)
		covCols := covariateColumns(header)
		central, lower, upper, level, sd := effectSummary(m)
		for _, r := range rows {
			row := Row{
				MarkID:              m.ID,
				Series:              string(m.Series),
				UnitID:              cell(r, idx, "unit_id"),
				UnitName:            cell(r, idx, "unit_name"),
				RunningValue:        parseFloat(cell(r, idx, "running_value")),
				Cutoff:              m.Design.Cutoff,
				Direction:           string(m.Design.Direction),
				Covariates:          covariateEntries(r, idx, covCols),
				Assigned:            cell(r, idx, "assigned") == "true",
				Treated:             parseOptBool(cell(r, idx, "treated")),
				Action:              m.Design.Action,
				Alternative:         m.Design.Alternative,
				EffectCentral:       central,
				EffectLower:         lower,
				EffectUpper:         upper,
				EffectIntervalLevel: level,
				EffectStdDev:        sd,
			}
			row.DistanceToCutoff = row.RunningValue - m.Design.Cutoff
			if o := cell(r, idx, "outcome"); o != "" {
				v := parseFloat(o)
				row.Outcome = &v
				row.OutcomeObserved = true
			}
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Series != out[j].Series {
			return out[i].Series < out[j].Series
		}
		if out[i].MarkID != out[j].MarkID {
			return out[i].MarkID < out[j].MarkID
		}
		return out[i].UnitID < out[j].UnitID
	})
	return out, nil
}

// WriteParquet writes the rows as a deterministic, single-row-group Parquet file
// at path and returns its content hash and size. Determinism rests on: a pinned
// CreatedBy string, a single row group, key-sorted covariate lists (built
// upstream in Build), and a fixed compression codec.
func WriteParquet(path string, rows []Row) (publish.WrittenArtifact, error) {
	var wa publish.WrittenArtifact
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return wa, err
	}
	f, err := os.Create(path)
	if err != nil {
		return wa, err
	}
	w := parquet.NewGenericWriter[Row](f,
		parquet.CreatedBy("openaction2outcome", schema.SchemaVersion, ""),
		parquet.Compression(&snappy.Codec{}),
		parquet.MaxRowsPerRowGroup(1<<62), // one row group: deterministic layout
	)
	if _, err := w.Write(rows); err != nil {
		f.Close()
		return wa, err
	}
	if err := w.Close(); err != nil {
		f.Close()
		return wa, err
	}
	if err := f.Close(); err != nil {
		return wa, err
	}
	sum, size, err := hashFileSize(path)
	if err != nil {
		return wa, err
	}
	return publish.WrittenArtifact{Path: path, SHA256: sum, Bytes: size, Rows: len(rows)}, nil
}

// Manifest is the slim, git-tracked pointer to the published episodes dataset —
// the dataset analogue of a mark: it carries enough to download and verify the
// bytes without re-running the mint.
type Manifest struct {
	SchemaVersion string   `json:"schema_version"`
	ID            string   `json:"id"`
	Format        string   `json:"format"`
	URI           string   `json:"uri"`
	SHA256        string   `json:"sha256"`
	Bytes         int64    `json:"bytes"`
	Rows          int      `json:"rows"`
	Columns       []string `json:"columns"`
	Series        []string `json:"series"`
	JoinKey       string   `json:"join_key"`
	Outcome       string   `json:"outcome"`
	Description   string   `json:"description"`
}

// NewManifest assembles the episodes manifest from a written artifact and the
// marks it was built from.
func NewManifest(wa publish.WrittenArtifact, uri string, marks []schema.Mark) Manifest {
	seen := map[string]bool{}
	var series []string
	for _, m := range marks {
		if s := string(m.Series); !seen[s] {
			seen[s] = true
			series = append(series, s)
		}
	}
	sort.Strings(series)
	return Manifest{
		SchemaVersion: schema.SchemaVersion,
		ID:            "episodes",
		Format:        "parquet",
		URI:           uri,
		SHA256:        wa.SHA256,
		Bytes:         wa.Bytes,
		Rows:          wa.Rows,
		Columns:       Columns,
		Series:        series,
		JoinKey:       "mark_id -> the mark's id (and the per-series Hugging Face configs)",
		Outcome:       "the later observed outcome; outcome_observed=false (outcome null) when a unit has no linked outcome",
		Description:   "Every mark's episode rows, unioned: one row per unit, giving its context before the decision (covariates, running_value/distance_to_cutoff), what was done (assigned/treated), and the outcome that followed — the (state, action, reward) view for model training. The full effect distribution stays in the marks, joinable on mark_id.",
	}
}

// WriteManifest writes the manifest as indented JSON (trailing newline) to path.
func WriteManifest(path string, mf Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// --- helpers.

func columnIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[h] = i
	}
	return idx
}

// covariateColumns returns the series-specific covariate column names in header
// order (header minus the fixed core columns).
func covariateColumns(header []string) []string {
	var cols []string
	for _, h := range header {
		if !coreColumns[h] {
			cols = append(cols, h)
		}
	}
	return cols
}

// covariateEntries builds the key-sorted covariate list for one row, skipping
// covariates that are empty (absent) for that unit.
func covariateEntries(row []string, idx map[string]int, covCols []string) []Covariate {
	var cv []Covariate
	for _, c := range covCols {
		v := cell(row, idx, c)
		if v == "" {
			continue
		}
		cv = append(cv, Covariate{Name: c, Value: parseFloat(v)})
	}
	sort.Slice(cv, func(i, j int) bool { return cv[i].Name < cv[j].Name })
	return cv
}

func effectSummary(m schema.Mark) (central, lower, upper, level, sd float64) {
	central = m.Effect.Central
	if m.Effect.Interval != nil {
		level, lower, upper = m.Effect.Interval.Level, m.Effect.Interval.Lower, m.Effect.Interval.Upper
	}
	if m.Effect.StdDev != nil {
		sd = *m.Effect.StdDev
	}
	return
}

func cell(row []string, idx map[string]int, name string) string {
	i, ok := idx[name]
	if !ok || i >= len(row) {
		return ""
	}
	return row[i]
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseOptBool(s string) *bool {
	if s == "" {
		return nil
	}
	b := s == "true"
	return &b
}

func hashFileSize(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
