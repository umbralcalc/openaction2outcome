package ingest

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

const sampleCSV = "\ufeffRECTYPE,URN,SCHNAME,NFTYPE,P8MEA,P8CIUPP,ATT8SCR,KS2APS,PTFSM6CLA1A,TPUP\n" +
	"1,100,Alpha School,ACC,0.3,0.5,48.2,28.1,21%,150\n" + // numeric -> kept
	"1,101,Beta School,ACC,NP,NP,NP,27.0,30%,120\n" + // P8 not numeric -> dropped
	"2,999,Some LA,,1.0,1.2,50,29,10%,9999\n" + // not a school record -> dropped
	"1,103,Gamma School,CY,-0.6,-0.1,44.0,26.5,57%,90\n" // numeric -> kept (below a -0.5 floor)

func TestLoadKS4FiltersAndParses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ks4.csv")
	if err := os.WriteFile(path, []byte(sampleCSV), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := LoadKS4(path)
	if err != nil {
		t.Fatalf("LoadKS4: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 mainstream numeric-P8 rows, got %d", len(rows))
	}
	byURN := map[string]SchoolKS4{}
	for _, r := range rows {
		byURN[r.URN] = r
	}
	a, ok := byURN["100"]
	if !ok {
		t.Fatal("URN 100 missing")
	}
	if a.P8 != 0.3 || a.Name != "Alpha School" {
		t.Errorf("URN 100 parsed wrong: %+v", a)
	}
	if a.PctFSM != 21 { // "21%" -> 21
		t.Errorf("percent parse: got %v want 21", a.PctFSM)
	}
	if a.KS2APS != 28.1 {
		t.Errorf("KS2APS parse: got %v", a.KS2APS)
	}
	if _, dropped := byURN["101"]; dropped {
		t.Error("row with non-numeric P8MEA should be dropped")
	}
}

func TestLoadKS4MissingColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.csv")
	os.WriteFile(path, []byte("URN,SCHNAME\n100,Alpha\n"), 0o644) // no RECTYPE/P8MEA
	if _, err := LoadKS4(path); err == nil {
		t.Fatal("expected error for missing required columns")
	}
}

func TestParseHelpers(t *testing.T) {
	if v := parsePercentOrNaN("57%"); v != 57 {
		t.Errorf("parsePercent 57%%: %v", v)
	}
	if v := parseNumOrNaN("12.5"); v != 12.5 {
		t.Errorf("parseNum 12.5: %v", v)
	}
	if v := parseNumOrNaN("SUPP"); !math.IsNaN(v) {
		t.Errorf("parseNum SUPP should be NaN, got %v", v)
	}
}

func TestVerifySHA(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("hello"), 0o644)
	// sha256("hello")
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if err := VerifySHA(path, want); err != nil {
		t.Fatalf("VerifySHA on correct hash: %v", err)
	}
	if err := VerifySHA(path, "0000"); err == nil {
		t.Fatal("expected mismatch error")
	}
	if err := VerifySHA(filepath.Join(dir, "nope"), want); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadSourceAndCachePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SOURCE.json")
	os.WriteFile(path, []byte(`{
      "source_id":"ks4-2015-2016-final",
      "download_uri":"https://gov/ks4.csv",
      "r2_object_key":"raw/ks4-2015-2016-final/ks4.csv",
      "local_path":"ks4.csv",
      "sha256":"abc123"
    }`), 0o644)
	s, err := LoadSource(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.SourceID != "ks4-2015-2016-final" || s.SHA256 != "abc123" {
		t.Errorf("bad source: %+v", s)
	}
	if got, want := s.CachePath("/cache"), filepath.Join("/cache", "ks4-2015-2016-final", "ks4.csv"); got != want {
		t.Errorf("CachePath: got %q want %q", got, want)
	}
}

func TestCandidateURLs(t *testing.T) {
	s := Source{R2ObjectKey: "raw/x/f.csv", DownloadURI: "https://gov/f.csv"}

	// no mirror base -> source only.
	if got := candidateURLs(s, ""); len(got) != 1 || got[0] != s.DownloadURI {
		t.Errorf("empty base: %v", got)
	}
	// placeholder base is not "real" -> source only.
	if got := candidateURLs(s, "https://REPLACE-WITH-R2.example"); len(got) != 1 || got[0] != s.DownloadURI {
		t.Errorf("placeholder base: %v", got)
	}
	// real base -> mirror first, then source fallback.
	got := candidateURLs(s, "https://base.r2.dev/")
	if len(got) != 2 || got[0] != "https://base.r2.dev/raw/x/f.csv" || got[1] != s.DownloadURI {
		t.Errorf("real base: %v", got)
	}
}

func TestFetchCacheHitIsOffline(t *testing.T) {
	cache := t.TempDir()
	// sha256("hello")
	const sum = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	s := Source{SourceID: "demo", LocalPath: "f.txt", SHA256: sum, DownloadURI: "https://unreachable.invalid/x"}
	dst := s.CachePath(cache)
	os.MkdirAll(filepath.Dir(dst), 0o755)
	os.WriteFile(dst, []byte("hello"), 0o644)
	// Already cached and correct: must return nil without touching the network.
	if err := Fetch(s, cache, ""); err != nil {
		t.Fatalf("cache-hit Fetch should be a no-op, got %v", err)
	}
}
