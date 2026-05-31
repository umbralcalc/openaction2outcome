package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is two levels up from internal/site.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

// genSite generates the real repo's site into a temp dir and returns it.
func genSite(t *testing.T) (root, out string) {
	t.Helper()
	root = repoRoot(t)
	out = t.TempDir()
	cfg := Config{
		MarksDir:      filepath.Join(root, "marks"),
		DossiersDir:   filepath.Join(root, "dossiers"),
		SchemaDoc:     filepath.Join(root, "docs", "schema.md"),
		ChangelogDoc:  filepath.Join(root, "CHANGELOG.md"),
		StudyPath:     filepath.Join(root, "scores", "calibration-study.json"),
		ManifestPath:  filepath.Join(root, "datasets", "episodes.manifest.json"),
		RawDir:        filepath.Join(root, "data", "raw"),
		PublishConfig: filepath.Join(root, "publish.json"),
		LogoPath:      filepath.Join(root, "assets", "logo.png"),
		OutDir:        out,
		RepoURL:       "https://github.com/umbralcalc/openaction2outcome",
		HFRepo:        "umbralcalc/openaction2outcome",
	}
	if err := Generate(cfg); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return root, out
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGenerateWritesExpectedFiles(t *testing.T) {
	_, out := genSite(t)
	for _, f := range []string{
		".nojekyll", "style.css", "index.html", "downloads.html", "schema.html",
		"changelog.html", "assets/logo.png", "downloads/marks.zip",
		"dossiers/index.html", "dossiers/bathing-water-poor-2015.html",
	} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("expected %s: %v", f, err)
		}
	}
}

func TestDownloadsPageHasManifestData(t *testing.T) {
	_, out := genSite(t)
	dl := read(t, filepath.Join(out, "downloads.html"))
	// The episodes manifest's content hash and URL must appear so a consumer can
	// verify the download.
	for _, want := range []string{
		"825022ea6303ec3f17e2fe5f97a59dd4fad9b4f0e76053027eb7253c066ecfeb",
		"episodes.parquet",
		"huggingface.co/datasets/umbralcalc/openaction2outcome",
		"downloads/marks.zip",
	} {
		if !strings.Contains(dl, want) {
			t.Errorf("downloads.html missing %q", want)
		}
	}
}

func TestDossierPageRendersTablesAndEffect(t *testing.T) {
	_, out := genSite(t)
	d := read(t, filepath.Join(out, "dossiers", "bathing-water-poor-2015.html"))
	if !strings.Contains(d, "<table>") {
		t.Error("dossier should render markdown tables to HTML")
	}
	if !strings.Contains(d, "Validity checks") {
		t.Error("dossier should carry its validity section")
	}
}

// No intra-repo markdown link should survive as a relative .md href: every link
// is either rewritten to a generated page or to an absolute GitHub URL.
func TestNoLeakedRelativeMarkdownLinks(t *testing.T) {
	_, out := genSite(t)
	pages, _ := filepath.Glob(filepath.Join(out, "*.html"))
	dossiers, _ := filepath.Glob(filepath.Join(out, "dossiers", "*.html"))
	for _, p := range append(pages, dossiers...) {
		html := read(t, p)
		// A relative .md href (not the absolute GitHub one) is a leak: every
		// intra-repo markdown link must be rewritten to a generated page or to
		// an absolute GitHub URL.
		for _, line := range strings.Split(html, "\n") {
			if i := strings.Index(line, `href="`); i >= 0 {
				rest := line[i+6:]
				if j := strings.Index(rest, `"`); j >= 0 {
					href := rest[:j]
					if strings.HasSuffix(href, ".md") && !strings.HasPrefix(href, "http") {
						t.Errorf("%s leaked relative markdown link %q", filepath.Base(p), href)
					}
				}
			}
		}
	}
}

func TestMarksZipIsDeterministic(t *testing.T) {
	root := repoRoot(t)
	marks, err := loadMarks(filepath.Join(root, "marks"))
	if err != nil {
		t.Fatal(err)
	}
	b1, s1, err := buildMarksZip(filepath.Join(root, "marks"), marks)
	if err != nil {
		t.Fatal(err)
	}
	b2, s2, err := buildMarksZip(filepath.Join(root, "marks"), marks)
	if err != nil {
		t.Fatal(err)
	}
	if s1 != s2 || len(b1) != len(b2) {
		t.Errorf("marks.zip not reproducible: %s (%d) vs %s (%d)", s1, len(b1), s2, len(b2))
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{0: "0 B", 512: "512 B", 1024: "1.0 KB", 204793: "200.0 KB"}
	for n, want := range cases {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
