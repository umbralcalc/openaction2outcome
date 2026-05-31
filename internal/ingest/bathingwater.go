package ingest

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// BathingWater is one designated English bathing water's annual compliance
// record for one season year under the revised Bathing Water Directive (rBWD)
// regime. The fields are the normalised projection of the frozen API harvest
// (scripts/bathingwater_harvest.py): the per-season classification plus the
// published log-normal percentile statistics from which the running variable
// (the Poor/Sufficient compliance margin) is reconstructed.
type BathingWater struct {
	SiteID              string
	EUBWID              string
	Name                string
	WaterType           string // "coastal" | "inland" (sets which thresholds apply)
	Region              string
	District            string
	SewerageUndertaker  string
	YearDesignated      string
	ImpactedByHeavyRain bool
	SeasonYear          int

	// ClassificationCode is the rBWD code: 1 Excellent, 2 Good, 3 Sufficient,
	// 4 Poor, 5 Insufficiently sampled, 6 New, 11 Closed.
	ClassificationCode int
	ClassificationName string

	FirstSampleDate string
	FinalSampleDate string

	// Compliance statistics over the rolling 4-year window, per indicator. The
	// 90th percentile is the binding statistic for the Poor/Sufficient boundary.
	ECp90, ECp95           float64
	ECn                    float64 // sample count used (post-discounting)
	ECmeanLog10, ECsdLog10 float64
	IEp90, IEp95           float64
	IEn                    float64
	IEmeanLog10, IEsdLog10 float64
}

// HasPercentiles reports whether both indicators carry a usable 90th-percentile
// statistic (i.e. the site-year was sufficiently sampled to be classified on the
// running variable). Insufficiently-sampled / New / Closed rows lack these.
func (b BathingWater) HasPercentiles() bool {
	return !math.IsNaN(b.ECp90) && b.ECp90 > 0 && !math.IsNaN(b.IEp90) && b.IEp90 > 0
}

// LoadBathingWater reads the frozen compliance+statistics CSV assembled by the
// harvest script and returns one record per (sampling point, season year).
func LoadBathingWater(csvPath string) ([]BathingWater, error) {
	rows, col, err := readCSV(csvPath)
	if err != nil {
		return nil, err
	}
	need := func(name string) error {
		if _, ok := col[name]; !ok {
			return fmt.Errorf("bathing-water: missing expected column %q", name)
		}
		return nil
	}
	for _, c := range []string{"site_id", "season_year", "classification_code", "ec_p90", "ie_p90"} {
		if err := need(c); err != nil {
			return nil, err
		}
	}
	get := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}

	out := make([]BathingWater, 0, len(rows))
	for _, rec := range rows {
		yr, ok := parseNum(get(rec, "season_year"))
		if !ok {
			continue
		}
		code := 0
		if c, ok := parseNum(get(rec, "classification_code")); ok {
			code = int(c + 0.5)
		}
		out = append(out, BathingWater{
			SiteID:              get(rec, "site_id"),
			EUBWID:              get(rec, "eubwid"),
			Name:                get(rec, "name"),
			WaterType:           strings.ToLower(strings.TrimSpace(get(rec, "water_type"))),
			Region:              get(rec, "region"),
			District:            get(rec, "district"),
			SewerageUndertaker:  get(rec, "sewerage_undertaker"),
			YearDesignated:      get(rec, "year_designated"),
			ImpactedByHeavyRain: parseBoolLoose(get(rec, "impacted_by_heavy_rain")),
			SeasonYear:          int(yr + 0.5),
			ClassificationCode:  code,
			ClassificationName:  get(rec, "classification_name"),
			FirstSampleDate:     get(rec, "first_sample_date"),
			FinalSampleDate:     get(rec, "final_sample_date"),
			ECp90:               parseNumOrNaN(get(rec, "ec_p90")),
			ECp95:               parseNumOrNaN(get(rec, "ec_p95")),
			ECn:                 parseNumOrNaN(get(rec, "ec_sample_count")),
			ECmeanLog10:         parseNumOrNaN(get(rec, "ec_mean_log10")),
			ECsdLog10:           parseNumOrNaN(get(rec, "ec_sd_log10")),
			IEp90:               parseNumOrNaN(get(rec, "ie_p90")),
			IEp95:               parseNumOrNaN(get(rec, "ie_p95")),
			IEn:                 parseNumOrNaN(get(rec, "ie_sample_count")),
			IEmeanLog10:         parseNumOrNaN(get(rec, "ie_mean_log10")),
			IEsdLog10:           parseNumOrNaN(get(rec, "ie_sd_log10")),
		})
	}
	return out, nil
}

// BathingWaterSample is one microbiology sample within a site's season, carrying
// the per-sample E. coli / intestinal enterococci counts and the discountable
// flag. These back the abnormal-sample-exclusion sensitivity check (re-including
// discountable samples and re-deriving the percentile near the cutoff).
type BathingWaterSample struct {
	SiteID         string
	SampleYear     int
	SampleDateTime string
	ECCount        float64
	ECQualifier    string // "<" (detection-limit) / ">" / ""
	IECount        float64
	IEQualifier    string
	Discountable   bool
}

// LoadBathingWaterSamples reads the frozen per-sample CSV (near-cutoff windows
// only) assembled by the harvest script.
func LoadBathingWaterSamples(csvPath string) ([]BathingWaterSample, error) {
	rows, col, err := readCSV(csvPath)
	if err != nil {
		return nil, err
	}
	for _, c := range []string{"site_id", "sample_year", "ec_count", "ie_count", "discountable"} {
		if _, ok := col[c]; !ok {
			return nil, fmt.Errorf("bathing-water samples: missing expected column %q", c)
		}
	}
	get := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}
	out := make([]BathingWaterSample, 0, len(rows))
	for _, rec := range rows {
		yr, ok := parseNum(get(rec, "sample_year"))
		if !ok {
			continue
		}
		out = append(out, BathingWaterSample{
			SiteID:         get(rec, "site_id"),
			SampleYear:     int(yr + 0.5),
			SampleDateTime: get(rec, "sample_date_time"),
			ECCount:        parseNumOrNaN(get(rec, "ec_count")),
			ECQualifier:    get(rec, "ec_qualifier"),
			IECount:        parseNumOrNaN(get(rec, "ie_count")),
			IEQualifier:    get(rec, "ie_qualifier"),
			Discountable:   parseBoolLoose(get(rec, "discountable")),
		})
	}
	return out, nil
}

// readCSV opens a CSV, strips a UTF-8 BOM, and returns the data rows plus a
// header-name→index map. Shared by the bathing-water loaders.
func readCSV(path string) ([][]string, map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, nil, err
	}
	if len(head) > 0 {
		head[0] = strings.TrimPrefix(head[0], "\ufeff")
	}
	col := indexColumns(head)
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, append([]string(nil), rec...))
	}
	return rows, col, nil
}

// parseBoolLoose parses "true"/"false"/"1"/"0"/"" tolerantly (the harvest writes
// linked-data boolean literals); anything unrecognised is false.
func parseBoolLoose(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		return true
	default:
		b, err := strconv.ParseBool(strings.TrimSpace(s))
		return err == nil && b
	}
}
