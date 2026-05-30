package ingest

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeSHMIZip(t *testing.T, csv string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "shmi.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("SHMI data/Historical_trust_level_SHMI_data_test_csv.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(csv)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return path
}

func TestLoadSHMI(t *testing.T) {
	csv := "TIME_PERIOD,PUBLICATION_MONTH,PROVIDER_CODE,PROVIDER_NAME,SHMI_VALUE,SHMI_BANDING,OD_LL,OD_UL,OBSERVED,EXPECTED\n" +
		"APR22_MAR23,JUL_23,RCF,\"AIREDALE, NHS FT\",1.25,1.0,0.85,1.18,1000,800\n" + // banded higher (SHMI>OD_UL)
		"APR22_MAR23,JUL_23,RXX,As Expected Trust,0.95,2.0,0.85,1.18,900,950\n" +
		"APR22_MAR23,JUL_23,RZZ,Suppressed Trust,*,,0.85,1.18,*,*\n" // suppressed SHMI -> dropped

	rows, err := LoadSHMI(writeSHMIZip(t, csv))
	if err != nil {
		t.Fatalf("LoadSHMI: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 parsed rows (suppressed dropped), got %d", len(rows))
	}
	byCode := map[string]TrustSHMI{}
	for _, r := range rows {
		byCode[r.ProviderCode] = r
	}
	a := byCode["RCF"]
	if a.SHMI != 1.25 || a.ODUpper != 1.18 || a.Banding != 1 {
		t.Errorf("RCF parsed wrong: %+v", a)
	}
	if a.ProviderName != "AIREDALE, NHS FT" { // quoted comma preserved
		t.Errorf("provider name with comma mis-parsed: %q", a.ProviderName)
	}
	if a.SHMI <= a.ODUpper {
		t.Error("RCF should be above its upper control limit (flagged higher)")
	}
	if byCode["RXX"].SHMI >= byCode["RXX"].ODUpper {
		t.Error("RXX should be within its control limits")
	}
}

func TestLoadSHMIMissingFile(t *testing.T) {
	// A zip without the historical CSV must error clearly.
	dir := t.TempDir()
	path := filepath.Join(dir, "z.zip")
	f, _ := os.Create(path)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("SHMI data/something_else.csv")
	w.Write([]byte("a,b\n1,2\n"))
	zw.Close()
	f.Close()
	if _, err := LoadSHMI(path); err == nil {
		t.Fatal("expected error when the historical CSV is absent")
	}
}
