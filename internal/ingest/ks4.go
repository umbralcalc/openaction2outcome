// Package ingest holds per-series loaders plus the fetch-to-cache step. Inputs are
// NOT vendored into git: data/raw holds only SOURCE.json pointers, and the bytes
// are fetched on demand into a gitignored cache (data/cache) and verified
// against the recorded SHA-256. Loaders are pure functions over the cached bytes.
package ingest

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SchoolKS4 is one school's normalised KS4 record.
type SchoolKS4 struct {
	URN     string
	Name    string
	NFType  string
	P8      float64 // P8MEA — the running variable / outcome depending on year
	P8CIUpp float64 // upper 95% CI of P8 (NaN if absent)
	Att8    float64 // NaN if absent
	KS2APS  float64 // average KS2 prior attainment (NaN if absent)
	PctFSM  float64 // % disadvantaged (FSM) (NaN if absent)
	Cohort  float64 // pupils at end of KS4 (NaN if absent)
}

// LoadKS4 reads a DfE KS4 performance-tables CSV and returns the mainstream-
// school records (RECTYPE==1) that have a numeric Progress 8 score. Non-numeric
// placeholders (NP, SUPP, LOWCOV, blank) are treated as missing.
func LoadKS4(csvPath string) ([]SchoolKS4, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerate ragged rows
	r.ReuseRecord = true

	head, err := r.Read()
	if err != nil {
		return nil, err
	}
	if len(head) > 0 {
		head[0] = strings.TrimPrefix(head[0], "\ufeff") // strip UTF-8 BOM
	}
	col := indexColumns(head)
	need := func(name string) (int, error) {
		i, ok := col[name]
		if !ok {
			return 0, fmt.Errorf("ks4: missing expected column %q", name)
		}
		return i, nil
	}
	iRec, err := need("RECTYPE")
	if err != nil {
		return nil, err
	}
	iURN, err := need("URN")
	if err != nil {
		return nil, err
	}
	iP8, err := need("P8MEA")
	if err != nil {
		return nil, err
	}

	get := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}

	var out []SchoolKS4
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if iRec >= len(rec) || rec[iRec] != "1" {
			continue
		}
		if iP8 >= len(rec) || iURN >= len(rec) {
			continue
		}
		p8, ok := parseNum(rec[iP8])
		if !ok {
			continue
		}
		out = append(out, SchoolKS4{
			URN:     rec[iURN],
			Name:    get(rec, "SCHNAME"),
			NFType:  get(rec, "NFTYPE"),
			P8:      p8,
			P8CIUpp: parseNumOrNaN(get(rec, "P8CIUPP")),
			Att8:    parseNumOrNaN(get(rec, "ATT8SCR")),
			KS2APS:  parseNumOrNaN(get(rec, "KS2APS")),
			PctFSM:  parsePercentOrNaN(get(rec, "PTFSM6CLA1A")),
			Cohort:  parseNumOrNaN(get(rec, "TPUP")),
		})
	}
	return out, nil
}

func indexColumns(head []string) map[string]int {
	m := make(map[string]int, len(head))
	for i, h := range head {
		m[h] = i
	}
	return m
}

func parseNum(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseNumOrNaN(s string) float64 {
	if v, ok := parseNum(s); ok {
		return v
	}
	return math.NaN()
}

// parsePercentOrNaN parses values like "57%" (the form DfE uses for proportion
// columns such as PTFSM6CLA1A), returning the numeric percentage.
func parsePercentOrNaN(s string) float64 {
	return parseNumOrNaN(strings.TrimSuffix(strings.TrimSpace(s), "%"))
}

// Source mirrors a /data/raw/<id>/SOURCE.json manifest: a pointer to a frozen
// open-data input (canonical source URL + object-store mirror key + SHA-256),
// the single source of truth for licence and integrity metadata.
type Source struct {
	SourceID    string `json:"source_id"`
	Title       string `json:"title"`
	Publisher   string `json:"publisher"`
	LandingPage string `json:"landing_page"`
	DownloadURI string `json:"download_uri"`
	R2ObjectKey string `json:"r2_object_key"`
	LocalPath   string `json:"local_path"`
	SHA256      string `json:"sha256"`
	Bytes       int64  `json:"bytes"`
	Format      string `json:"format"`
	Licence     string `json:"licence"`
	LicenceURI  string `json:"licence_uri"`
	Vintage     string `json:"vintage"`
	Role        string `json:"role"`
}

// LoadSource reads a SOURCE.json manifest.
func LoadSource(path string) (Source, error) {
	var s Source
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("decode %s: %w", path, err)
	}
	return s, nil
}

// CachePath returns where a source's bytes live in the local cache.
func (s Source) CachePath(cacheDir string) string {
	return filepath.Join(cacheDir, s.SourceID, s.LocalPath)
}

// VerifySHA checks the SHA-256 of the file at path against want.
func VerifySHA(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("integrity check failed for %s: got %s, want %s", path, got, want)
	}
	return nil
}

// Fetch ensures a source's bytes are present and correct in the cache. If the
// cached file already matches the recorded SHA-256 it does nothing (offline).
// Otherwise it downloads — preferring the object-store mirror (mirrorBase +
// r2_object_key) when mirrorBase is set, else the canonical download_uri — then
// verifies the hash before committing the file. A non-empty, non-placeholder
// mirrorBase is treated as preferred.
func Fetch(s Source, cacheDir, mirrorBase string) error {
	dst := s.CachePath(cacheDir)
	if VerifySHA(dst, s.SHA256) == nil {
		return nil // already cached and correct
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	urls := candidateURLs(s, mirrorBase)
	var lastErr error
	for _, u := range urls {
		if err := download(u, dst); err != nil {
			lastErr = err
			continue
		}
		if err := VerifySHA(dst, s.SHA256); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("fetch %s: all sources failed: %w", s.SourceID, lastErr)
}

func candidateURLs(s Source, mirrorBase string) []string {
	var urls []string
	if isRealBase(mirrorBase) && s.R2ObjectKey != "" {
		urls = append(urls, strings.TrimRight(mirrorBase, "/")+"/"+s.R2ObjectKey)
	}
	if s.DownloadURI != "" {
		urls = append(urls, s.DownloadURI)
	}
	return urls
}

func isRealBase(b string) bool {
	return b != "" && !strings.Contains(b, "REPLACE-WITH")
}

func download(url, dst string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "openaction2outcome-fetch/1.0")
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
