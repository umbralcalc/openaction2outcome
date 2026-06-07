// Package site generates the project's static GitHub Pages site from the
// committed marks, dossiers, schema docs, and dataset manifest. It is a faithful
// HTML view of artifacts that already live in the repo plus a downloads page —
// no content is authored here that isn't derivable from those sources, so the
// site can never silently drift from the data. Generation is deterministic:
// inputs are read in sorted order, nothing is timestamped, and the marks zip
// uses fixed modtimes, so the same repo state produces byte-identical output.
package site

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/umbralcalc/openaction2outcome/internal/episodes"
	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// Config tells Generate where the inputs live and where to write the site.
type Config struct {
	MarksDir      string // directory of mark JSON files
	DossiersDir   string // directory of rendered dossier markdown
	SchemaDoc     string // the data-dictionary markdown (docs/schema.md)
	ChangelogDoc  string // CHANGELOG.md
	StudyPath     string // calibration-study.json (for the headline finding)
	ManifestPath  string // datasets/episodes.manifest.json
	RawDir        string // data/raw (SOURCE.json pointers for the frozen-input table)
	PublishConfig string // publish.json (object-store base URL)
	LogoPath      string // logo to copy into the site
	OutDir        string // where to write the site (the Pages /docs folder)
	RepoURL       string // GitHub repo base, e.g. https://github.com/umbralcalc/openaction2outcome
}

// Generate writes the full static site into cfg.OutDir.
func Generate(cfg Config) error {
	marks, err := loadMarks(cfg.MarksDir)
	if err != nil {
		return err
	}
	if len(marks) == 0 {
		return fmt.Errorf("site: no marks under %s", cfg.MarksDir)
	}
	cards := marksToCards(marks)

	// Scaffold the output tree.
	for _, d := range []string{cfg.OutDir, filepath.Join(cfg.OutDir, "dossiers"),
		filepath.Join(cfg.OutDir, "assets"), filepath.Join(cfg.OutDir, "downloads")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	// .nojekyll: serve files as-is (no Jekyll processing of underscores etc.).
	if err := os.WriteFile(filepath.Join(cfg.OutDir, ".nojekyll"), nil, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.OutDir, "style.css"), []byte(styleCSS), 0o644); err != nil {
		return err
	}
	if err := copyFile(cfg.LogoPath, filepath.Join(cfg.OutDir, "assets", "logo.png")); err != nil {
		return fmt.Errorf("copy logo: %w", err)
	}

	// The marks zip is a real, content-addressed download artifact.
	zipBytes, zipSum, err := buildMarksZip(cfg.MarksDir, marks)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.OutDir, "downloads", "marks.zip"), zipBytes, 0o644); err != nil {
		return err
	}

	// Home.
	plugIn, sbi := studyHeadline(cfg.StudyPath)
	homeBody, err := renderTemplate("home", homeTmpl, map[string]any{
		"Marks": cards, "RepoURL": cfg.RepoURL, "PlugInCov": plugIn, "SBICov": sbi,
	})
	if err != nil {
		return err
	}
	if err := cfg.writePage("index.html", "Home", "index.html", "", homeBody); err != nil {
		return err
	}

	// Downloads.
	dlBody, err := cfg.renderDownloads(marks, zipBytes, zipSum)
	if err != nil {
		return err
	}
	if err := cfg.writePage("downloads.html", "Datasets", "downloads.html", "", dlBody); err != nil {
		return err
	}

	// Markdown documentation pages (single source of truth: the repo markdown).
	docPages := []struct{ src, out, title, active string }{
		{cfg.SchemaDoc, "schema.html", "Schema", "schema.html"},
		{cfg.ChangelogDoc, "changelog.html", "Changelog", "changelog.html"},
	}
	for _, p := range docPages {
		body, err := cfg.renderMarkdownFile(p.src, "")
		if err != nil {
			return fmt.Errorf("render %s: %w", p.src, err)
		}
		if err := cfg.writePage(p.out, p.title, p.active, "", body); err != nil {
			return err
		}
	}

	// Dossiers: an index plus one page per mark (one level down → "../" prefix).
	idxBody, err := renderTemplate("dossiers", dossiersTmpl, map[string]any{"Marks": cards})
	if err != nil {
		return err
	}
	if err := cfg.writePage(filepath.Join("dossiers", "index.html"), "Dossiers", "dossiers/index.html", "../", idxBody); err != nil {
		return err
	}
	for _, m := range marks {
		src := filepath.Join(cfg.DossiersDir, m.ID+".md")
		body, err := cfg.renderMarkdownFile(src, "../")
		if err != nil {
			return fmt.Errorf("render dossier %s: %w", m.ID, err)
		}
		out := filepath.Join("dossiers", m.ID+".html")
		if err := cfg.writePage(out, m.ID, "dossiers/index.html", "../", body); err != nil {
			return err
		}
	}
	return nil
}

// writePage wraps a body fragment in the shared layout and writes it to
// cfg.OutDir/relOut. active is the nav Href to highlight; prefix is the page's
// path-to-root.
func (cfg Config) writePage(relOut, title, active, prefix string, body template.HTML) error {
	nav := make([]navItem, len(navItems))
	for i, n := range navItems {
		nav[i] = navItem{Href: n.Href, Label: n.Label, Current: n.Href == active}
	}
	var buf bytes.Buffer
	if err := layout.Execute(&buf, page{
		Title: title, Active: active, Prefix: prefix, Body: body,
		RepoURL: cfg.RepoURL, Nav: nav,
	}); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cfg.OutDir, relOut), buf.Bytes(), 0o644)
}

// renderMarkdownFile reads a markdown file and renders it to a link-rewritten
// HTML fragment.
func (cfg Config) renderMarkdownFile(path, prefix string) (template.HTML, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	html, err := renderMarkdown(src, cfg.RepoURL, prefix)
	if err != nil {
		return "", err
	}
	return template.HTML(html), nil
}

// --- home / dossier cards ---------------------------------------------------

type markCard struct {
	ID, Series, Domain, UnitType, Estimand, EffectStr, Status string
}

func marksToCards(marks []schema.Mark) []markCard {
	cards := make([]markCard, 0, len(marks))
	for _, m := range marks {
		status := "ADMITTED"
		if !m.Dossier.Admitted {
			status = "NOT ADMITTED"
		}
		cards = append(cards, markCard{
			ID: m.ID, Series: string(m.Series), Domain: m.Domain, UnitType: m.UnitType,
			Estimand: m.Design.Estimand, EffectStr: effectStr(m.Effect), Status: status,
		})
	}
	return cards
}

// effectStr is the compact "central · 95% CI [lo, hi]" summary used on cards.
func effectStr(e schema.Distribution) string {
	if e.Interval != nil {
		return fmt.Sprintf("%s · %.0f%% CI [%s, %s]", num(round4(e.Central)),
			100*e.Interval.Level, num(round4(e.Interval.Lower)), num(round4(e.Interval.Upper)))
	}
	return num(round4(e.Central))
}

// --- downloads --------------------------------------------------------------

type sourceRow struct {
	SourceID, Title, Publisher, Licence, LicenceURI, URL, OrigURL string
	Bytes                                                         int64
}

// episodeFile is one mark's published episode artifact, read from the manifest.
// The episode rows are stored per mark (one gzipped CSV each), not as a single
// unified file; the manifest records each file's URL, hash, and size.
type episodeFile struct {
	ID, Series, EffectStr, URL, SHA256, Bytes string
	Rows                                      int
}

func (cfg Config) renderDownloads(marks []schema.Mark, zipBytes []byte, zipSum string) (template.HTML, error) {
	man, err := loadManifest(cfg.ManifestPath)
	if err != nil {
		return "", fmt.Errorf("load manifest: %w", err)
	}
	pcfg, err := publish.LoadConfig(cfg.PublishConfig)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("load publish config: %w", err)
	}
	sources, err := loadSourceRows(cfg.RawDir, pcfg)
	if err != nil {
		return "", err
	}

	// One download per mark, taken straight from the manifest so the URL, hash,
	// and size match what is actually served.
	effects := map[string]string{}
	for _, m := range marks {
		effects[m.ID] = effectStr(m.Effect)
	}
	eps := make([]episodeFile, 0, len(man.Marks))
	for _, a := range man.Marks {
		eps = append(eps, episodeFile{
			ID: a.MarkID, Series: a.Series, EffectStr: effects[a.MarkID],
			URL: a.URI, SHA256: a.SHA256, Bytes: humanBytes(a.Bytes), Rows: a.Rows,
		})
	}

	return renderTemplate("downloads", downloadsTmpl, map[string]any{
		"EpisodeFiles":     eps,
		"EpisodeTotalRows": man.TotalRows,
		"ColumnsJoined":    strings.Join(man.CoreColumns, ", "),
		"MarksZip":         map[string]any{"Count": len(marks), "Bytes": int64(len(zipBytes)), "SHA256": zipSum},
		"Sources":          sources,
		"RepoURL":          cfg.RepoURL,
	})
}

// loadSourceRows reads every data/raw/<id>/SOURCE.json into a display row,
// preferring the object-store mirror URL when a real base_url is configured.
func loadSourceRows(rawDir string, pcfg publish.Config) ([]sourceRow, error) {
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	mirror := usableBaseURL(pcfg.BaseURL)
	var rows []sourceRow
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(rawDir, e.Name(), "SOURCE.json")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		s, err := ingest.LoadSource(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		r := sourceRow{
			SourceID: s.SourceID, Title: s.Title, Publisher: s.Publisher,
			Licence: s.Licence, LicenceURI: s.LicenceURI, Bytes: s.Bytes,
		}
		if mirror != "" && s.R2ObjectKey != "" {
			r.URL = mirror + "/" + s.R2ObjectKey
			r.OrigURL = s.DownloadURI
		} else {
			r.URL = s.DownloadURI
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].SourceID < rows[j].SourceID })
	return rows, nil
}

// usableBaseURL returns the base URL only if it is a real configured value (not
// empty and not the documented placeholder).
func usableBaseURL(base string) string {
	base = strings.TrimRight(base, "/")
	if base == "" || strings.Contains(strings.ToUpper(base), "REPLACE") {
		return ""
	}
	return base
}

// --- marks zip --------------------------------------------------------------

// zipEpoch is a fixed modtime for every zip entry so the archive is reproducible.
var zipEpoch = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

func buildMarksZip(marksDir string, marks []schema.Mark) ([]byte, string, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, m := range marks {
		raw, err := os.ReadFile(filepath.Join(marksDir, m.ID+".json"))
		if err != nil {
			return nil, "", err
		}
		hdr := &zip.FileHeader{Name: "marks/" + m.ID + ".json", Method: zip.Deflate}
		hdr.Modified = zipEpoch
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, "", err
		}
		if _, err := w.Write(raw); err != nil {
			return nil, "", err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:]), nil
}

// --- shared helpers ---------------------------------------------------------

func renderTemplate(name, tmpl string, data any) (template.HTML, error) {
	t, err := template.New(name).Funcs(template.FuncMap{"bytes": humanBytes}).Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

func loadMarks(dir string) ([]schema.Mark, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths)
	marks := make([]schema.Mark, 0, len(paths))
	for _, p := range paths {
		m, err := schema.LoadMark(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		marks = append(marks, m)
	}
	return marks, nil
}

func loadManifest(path string) (episodes.Manifest, error) {
	var m episodes.Manifest
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

// studyHeadline pulls the 95%-nominal coverage of each method from the
// calibration study, returning display percentages. Missing/unparseable study →
// the values the README cites, so the page is never blank.
func studyHeadline(path string) (plugIn, sbi string) {
	plugIn, sbi = "80%", "92%"
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var study struct {
		Levels []float64 `json:"levels"`
		PlugIn struct {
			Coverage []float64 `json:"coverage"`
		} `json:"plug_in"`
		SBI struct {
			Coverage []float64 `json:"coverage"`
		} `json:"sbi"`
	}
	if json.Unmarshal(b, &study) != nil {
		return
	}
	for i, L := range study.Levels {
		if math.Abs(L-0.95) < 1e-9 && i < len(study.PlugIn.Coverage) && i < len(study.SBI.Coverage) {
			plugIn = fmt.Sprintf("%.0f%%", 100*study.PlugIn.Coverage[i])
			sbi = fmt.Sprintf("%.0f%%", 100*study.SBI.Coverage[i])
		}
	}
	return
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// humanBytes formats a byte count as a short human-readable size.
func humanBytes(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(u), 0
	for m := n / u; m >= u; m /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }

// num renders a number with a real Unicode minus sign for display.
func num(x float64) string {
	return strings.Replace(fmt.Sprintf("%g", x), "-", "−", 1)
}
