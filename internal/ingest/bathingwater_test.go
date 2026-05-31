package ingest

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func writeCSV(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadBathingWater(t *testing.T) {
	csv := "site_id,eubwid,name,water_type,region,district,sewerage_undertaker,year_designated,impacted_by_heavy_rain,season_year,classification_code,classification_name,first_sample_date,final_sample_date,ec_p90,ec_p95,ec_sample_count,ec_mean_log10,ec_sd_log10,ie_p90,ie_p95,ie_sample_count,ie_mean_log10,ie_sd_log10\n" +
		"03600,ukc2102-03600,Spittal,Coastal,North East,Northumberland,02366703,1988,true,2024,2,Good,2021-05-06,2024-09-17,175.2,298.82,80,1.44,0.63,90,150,80,1.2,0.55\n" +
		// a Poor inland water (fails Sufficient) with all stats present
		"90500,uki2000-90500,River Test,Inland,South West,Testshire,09999999,2021,false,2024,4,Poor,2021-05-01,2024-09-20,1200,2100,75,2.5,0.7,400,800,75,2.3,0.6\n" +
		// an insufficiently-sampled row: percentiles blank -> HasPercentiles false
		"07777,ukc9999-07777,New Beach,Coastal,,,,,2024,5,InsufficientlySampled,,,,,,,,,,,,\n"

	rows, err := LoadBathingWater(writeCSV(t, "compliance.csv", csv))
	if err != nil {
		t.Fatalf("LoadBathingWater: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	byID := map[string]BathingWater{}
	for _, r := range rows {
		byID[r.SiteID] = r
	}

	sp := byID["03600"]
	if sp.WaterType != "coastal" {
		t.Errorf("water_type should normalise to lower-case coastal, got %q", sp.WaterType)
	}
	if sp.ClassificationCode != 2 || sp.ClassificationName != "Good" {
		t.Errorf("Spittal classification parsed wrong: %+v", sp)
	}
	if !sp.ImpactedByHeavyRain {
		t.Error("Spittal impacted_by_heavy_rain should be true")
	}
	if sp.ECp90 != 175.2 || sp.IEp90 != 90 || sp.ECn != 80 {
		t.Errorf("Spittal percentiles parsed wrong: %+v", sp)
	}
	if !sp.HasPercentiles() {
		t.Error("Spittal should have usable percentiles")
	}

	rt := byID["90500"]
	if rt.WaterType != "inland" || rt.ClassificationCode != 4 {
		t.Errorf("River Test parsed wrong: %+v", rt)
	}
	if rt.ImpactedByHeavyRain {
		t.Error("River Test impacted_by_heavy_rain should be false")
	}

	nb := byID["07777"]
	if nb.HasPercentiles() {
		t.Error("insufficiently-sampled site should report no usable percentiles")
	}
	if !math.IsNaN(nb.ECp90) {
		t.Errorf("blank ec_p90 should parse to NaN, got %v", nb.ECp90)
	}
}

func TestLoadBathingWaterSamples(t *testing.T) {
	csv := "site_id,sample_year,sample_date_time,ec_count,ec_qualifier,ie_count,ie_qualifier,discountable\n" +
		"90500,2024,2024-08-01T10:00:00,5000,>,1800,,true\n" + // a discountable (abnormal) spike
		"90500,2024,2024-07-15T09:30:00,40,<,15,<,false\n"

	rows, err := LoadBathingWaterSamples(writeCSV(t, "samples.csv", csv))
	if err != nil {
		t.Fatalf("LoadBathingWaterSamples: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 sample rows, got %d", len(rows))
	}
	if !rows[0].Discountable || rows[0].ECCount != 5000 || rows[0].ECQualifier != ">" {
		t.Errorf("discountable spike parsed wrong: %+v", rows[0])
	}
	if rows[1].Discountable || rows[1].IEQualifier != "<" {
		t.Errorf("normal sample parsed wrong: %+v", rows[1])
	}
}

func TestLoadBathingWaterMissingColumn(t *testing.T) {
	if _, err := LoadBathingWater(writeCSV(t, "bad.csv", "site_id,season_year\n03600,2024\n")); err == nil {
		t.Fatal("expected error when required statistic columns are absent")
	}
}
