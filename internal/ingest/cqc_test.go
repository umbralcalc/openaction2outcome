package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCQCInspections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cqc.csv")
	os.WriteFile(path, []byte(
		"provider_ods,provider_name,report_date,report_type\n"+
			"RCF,Airedale,2019-03-14,Provider\n"+
			"RCF,Airedale,2017-09-20,Provider\n"+ // out of order on purpose
			"RXX,Other,2020-06-01,CoreService\n"+
			"RZZ,NoDate,,Provider\n"), 0o644) // empty date -> skipped

	c, err := LoadCQCInspections(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c["RCF"]) != 2 {
		t.Fatalf("RCF should have 2 dated reports, got %v", c["RCF"])
	}
	if c["RCF"][0] != "2017-09-20" || c["RCF"][1] != "2019-03-14" {
		t.Errorf("dates should be sorted ascending: %v", c["RCF"])
	}
	if _, ok := c["RZZ"]; ok {
		t.Error("a row with an empty date should be skipped")
	}
	if !c.InspectedBetween("RCF", "2019-01-01", "2020-01-01") {
		t.Error("RCF was inspected in 2019")
	}
	if c.InspectedBetween("RCF", "2021-01-01", "2022-01-01") {
		t.Error("RCF was not inspected in 2021")
	}
	if c.InspectedBetween("ZZZ", "2000-01-01", "2030-01-01") {
		t.Error("unknown trust should never be inspected")
	}
}

func TestLoadCQCMissingColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.csv")
	os.WriteFile(path, []byte("a,b\n1,2\n"), 0o644)
	if _, err := LoadCQCInspections(path); err == nil {
		t.Fatal("expected error for missing required columns")
	}
}
