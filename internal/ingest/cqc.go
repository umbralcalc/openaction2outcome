package ingest

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// CQCReports maps an NHS trust ODS code to its CQC report (inspection) dates,
// each "YYYY-MM-DD", sorted ascending.
type CQCReports map[string][]string

// LoadCQCInspections reads the frozen CQC report-events CSV
// (provider_ods, provider_name, report_date, report_type).
func LoadCQCInspections(path string) (CQCReports, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, err
	}
	if len(head) > 0 {
		head[0] = strings.TrimPrefix(head[0], "\ufeff")
	}
	col := indexColumns(head)
	iODS, ok1 := col["provider_ods"]
	iDate, ok2 := col["report_date"]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("cqc: missing provider_ods/report_date columns")
	}

	out := make(CQCReports)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if iODS >= len(rec) || iDate >= len(rec) {
			continue
		}
		if rec[iDate] == "" {
			continue
		}
		out[rec[iODS]] = append(out[rec[iODS]], rec[iDate])
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out, nil
}

// InspectedBetween reports whether the trust has any CQC report dated in
// [start, end) (date strings "YYYY-MM-DD").
func (c CQCReports) InspectedBetween(ods, start, end string) bool {
	for _, d := range c[ods] {
		if d >= start && d < end {
			return true
		}
	}
	return false
}
