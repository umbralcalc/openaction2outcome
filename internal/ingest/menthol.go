package ingest

import (
	"fmt"
	"math"
	"strings"
)

// SmokingObs is one (province × year) adult current-smoking prevalence observation
// from the frozen StatCan CCHS panel (scripts/menthol_harvest.py). Pct is the
// "Current smoker, daily or occasional", "Total, 12 years and over" percentage; Lo
// and Hi are its 95% confidence-interval bounds (NaN when StatCan suppressed them),
// from which a per-cell standard error is derived. SourceTable records which StatCan
// table the row came from (13100451 pre-2015, 13100096 2015+), so the 2015 CCHS
// redesign break is inspectable.
type SmokingObs struct {
	Province    string // two-letter code (ON, QC, BC, …)
	Year        int
	Pct         float64
	Lo, Hi      float64 // 95% CI bounds on the percentage (NaN if absent)
	SourceTable string
}

// SE returns the standard error implied by the 95% CI ((Hi-Lo)/(2*1.96)); NaN when
// the CI is absent.
func (s SmokingObs) SE() float64 {
	if math.IsNaN(s.Lo) || math.IsNaN(s.Hi) || s.Hi < s.Lo {
		return math.NaN()
	}
	return (s.Hi - s.Lo) / (2 * 1.959963984540054)
}

// LoadSmokingPanel reads the frozen province×year smoking panel.
func LoadSmokingPanel(csvPath string) ([]SmokingObs, error) {
	rows, col, err := readCSV(csvPath)
	if err != nil {
		return nil, err
	}
	for _, c := range []string{"province", "year", "smoking_pct"} {
		if _, ok := col[c]; !ok {
			return nil, fmt.Errorf("smoking panel: missing expected column %q", c)
		}
	}
	get := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}
	out := make([]SmokingObs, 0, len(rows))
	for _, rec := range rows {
		yr, ok := parseNum(get(rec, "year"))
		if !ok {
			continue
		}
		pct := parseNumOrNaN(get(rec, "smoking_pct"))
		if math.IsNaN(pct) {
			continue
		}
		out = append(out, SmokingObs{
			Province:    strings.ToUpper(strings.TrimSpace(get(rec, "province"))),
			Year:        int(yr + 0.5),
			Pct:         pct,
			Lo:          parseNumOrNaN(get(rec, "smoking_lo")),
			Hi:          parseNumOrNaN(get(rec, "smoking_hi")),
			SourceTable: get(rec, "source_table"),
		})
	}
	return out, nil
}
