// Package episodes reshapes every mark's per-unit rows into the single published
// `episodes` dataset: one row per unit, across all series, in the terms the marks
// already use — the unit's context before the decision (its covariates and where
// it sat relative to the cutoff), what was done to it (assigned / treated), and
// the outcome that followed. That is the (state, action, reward) view a model
// trainer wants, named the way the rest of the project names things.
//
// The published form is per mark: each mark's rows are one gzipped CSV
// (dist/marks/<id>/episodes.csv.gz) served from object storage at
// marks/<id>/episodes.csv.gz, and the slim git-tracked manifest lists every
// mark's file with its sha256 + size (see NewManifest). A consumer downloads a
// mark's file and joins its rows to the mark JSON on the mark id for the design
// (cutoff, direction, action) and the full effect distribution. The same per-mark
// CSVs are mirrored into the Hugging Face dataset directory (same schema), so
// there is one row shape everywhere — no unioned re-encoding.
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

// LoadTable reads the episode rows a build staged for the given mark under
// distDir/marks/<id>/episodes.csv.gz and returns the parsed header and rows. It
// is offline-first: the rows must already be staged (run `openaction2outcome
// build` first). The staged file is the deterministic artifact that gets
// published; the manifest records its SHA-256 so a consumer's download verifies.
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

// CopyToHF mirrors every mark's staged episodes.csv.gz into the Hugging Face
// dataset directory at hfDir/episodes/<id>.csv.gz, so a Hugging Face user can pull
// a mark's rows directly (same schema as the object-storage file). It returns the
// repo-relative paths written, in mark order.
func CopyToHF(marks []schema.Mark, distDir, hfDir string) ([]string, error) {
	outDir := filepath.Join(hfDir, "episodes")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	var written []string
	for _, m := range marks {
		src := filepath.Join(distDir, "marks", m.ID, stagedTableName)
		b, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("mark %q episodes not staged at %s (run `openaction2outcome build`): %w", m.ID, src, err)
		}
		rel := filepath.Join("episodes", m.ID+".csv.gz")
		if err := os.WriteFile(filepath.Join(hfDir, rel), b, 0o644); err != nil {
			return nil, err
		}
		written = append(written, rel)
	}
	return written, nil
}

// CoreColumns are the fixed leading columns of every per-mark episodes.csv.gz,
// in order. Each mark's file appends its own series-specific covariate columns
// after these (recorded per mark in the manifest).
var CoreColumns = []string{"unit_id", "unit_name", "running_value", "assigned", "treated", "outcome"}

// MarkArtifact points at one mark's published episodes.csv.gz and carries enough
// to download and verify it without re-running the mint.
type MarkArtifact struct {
	MarkID string `json:"mark_id"`
	Series string `json:"series"`
	// RowShape is the mark's episode-row shape: "cross-section" (RDD/DiD, one row
	// per unit) or "panel" (ITS, one row per series × time bucket). A reader keys
	// its row decoder off it.
	RowShape   string   `json:"row_shape"`
	URI        string   `json:"uri"`
	SHA256     string   `json:"sha256"`
	Bytes      int64    `json:"bytes"`
	Rows       int      `json:"rows"`
	Covariates []string `json:"covariates"`
}

// Manifest is the slim, git-tracked pointer to the published episodes dataset.
// The episode rows are published per mark — one gzipped CSV each — so the
// manifest lists every mark's artifact (URL + sha256 + size) rather than a single
// unified file. Each row is (unit_id, unit_name, running_value, assigned,
// treated, outcome, + the mark's covariates); join to the mark on its id for the
// design (cutoff, direction, action) and the full effect distribution.
type Manifest struct {
	SchemaVersion string         `json:"schema_version"`
	ID            string         `json:"id"`
	Format        string         `json:"format"`
	CoreColumns   []string       `json:"core_columns"`
	TotalRows     int            `json:"total_rows"`
	Series        []string       `json:"series"`
	JoinKey       string         `json:"join_key"`
	Outcome       string         `json:"outcome"`
	Description   string         `json:"description"`
	Marks         []MarkArtifact `json:"marks"`
}

// NewManifest assembles the per-mark episodes manifest by hashing each mark's
// staged episodes.csv.gz under distDir and resolving its public URL from cfg.
// The marks must already be built (their rows staged); the same bytes are what
// publish.sh uploads, so the recorded sha256 verifies a consumer's download.
func NewManifest(marks []schema.Mark, distDir string, cfg publish.Config) (Manifest, error) {
	seen := map[string]bool{}
	var series []string
	arts := make([]MarkArtifact, 0, len(marks))
	total := 0
	for _, m := range marks {
		header, rows, err := LoadTable(m, distDir)
		if err != nil {
			return Manifest{}, err
		}
		path := filepath.Join(distDir, "marks", m.ID, stagedTableName)
		sum, size, err := hashFileSize(path)
		if err != nil {
			return Manifest{}, err
		}
		s := string(m.Series)
		if !seen[s] {
			seen[s] = true
			series = append(series, s)
		}
		total += len(rows)
		arts = append(arts, MarkArtifact{
			MarkID:     m.ID,
			Series:     s,
			RowShape:   string(m.EffectiveRowShape()),
			URI:        cfg.MarkArtifactURL(m.ID, stagedTableName),
			SHA256:     sum,
			Bytes:      size,
			Rows:       len(rows),
			Covariates: covariateColumns(header),
		})
	}
	sort.Strings(series)
	sort.Slice(arts, func(i, j int) bool { return arts[i].MarkID < arts[j].MarkID })
	return Manifest{
		SchemaVersion: schema.SchemaVersion,
		ID:            "episodes",
		Format:        "csv.gz",
		CoreColumns:   CoreColumns,
		TotalRows:     total,
		Series:        series,
		JoinKey:       "mark_id -> the mark's id (the file is keyed by mark; join its rows to the mark JSON, and to the per-series Hugging Face configs)",
		Outcome:       "the later observed outcome; the outcome cell is empty when a unit has no linked outcome (e.g. attrition)",
		Description:   "Each mark's per-unit episode rows, published as one gzipped CSV per mark: a unit's context before the decision (covariates, running_value), what was done (assigned/treated), and the outcome that followed — the (state, action, reward) view for model training. The design (cutoff, direction, action) and the full effect distribution stay in the mark JSON, joinable on the mark id.",
		Marks:         arts,
	}, nil
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
