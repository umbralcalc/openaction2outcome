package ingest

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// TrustSHMI is one NHS trust's SHMI record for one rolling 12-month window.
type TrustSHMI struct {
	ProviderCode     string
	ProviderName     string
	TimePeriod       string // reporting window, e.g. "APR22_MAR23"
	PublicationMonth string // when this window was published, e.g. "JUL_23"
	SHMI             float64
	ODUpper          float64 // overdispersed upper control limit (the band-1 cutoff)
	ODLower          float64 // overdispersed lower control limit
	Banding          int     // 1 = higher than expected, 2 = as expected, 3 = lower
	Observed         float64
	Expected         float64
}

// LoadSHMI reads the historical trust-level SHMI table from the published data
// zip. It returns one record per trust × reporting window. Rows with a
// non-numeric SHMI or control limit (suppressed "*") are skipped.
func LoadSHMI(zipPath string) ([]TrustSHMI, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var entry *zip.File
	for _, f := range zr.File {
		if strings.Contains(f.Name, "Historical_trust_level_SHMI_data") && strings.HasSuffix(f.Name, ".csv") {
			entry = f
			break
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("shmi: historical trust-level CSV not found in %s", zipPath)
	}

	rc, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	r := csv.NewReader(rc)
	r.FieldsPerRecord = -1
	r.ReuseRecord = true

	head, err := r.Read()
	if err != nil {
		return nil, err
	}
	if len(head) > 0 {
		head[0] = strings.TrimPrefix(head[0], "\ufeff")
	}
	col := indexColumns(head)
	idx := func(name string) (int, error) {
		i, ok := col[name]
		if !ok {
			return 0, fmt.Errorf("shmi: missing column %q", name)
		}
		return i, nil
	}
	cProv, e1 := idx("PROVIDER_CODE")
	cSHMI, e2 := idx("SHMI_VALUE")
	cODU, e3 := idx("OD_UL")
	for _, e := range []error{e1, e2, e3} {
		if e != nil {
			return nil, e
		}
	}

	get := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}

	var out []TrustSHMI
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if cProv >= len(rec) || cSHMI >= len(rec) || cODU >= len(rec) {
			continue
		}
		shmi, ok := parseNum(rec[cSHMI])
		if !ok {
			continue
		}
		odu, ok := parseNum(rec[cODU])
		if !ok {
			continue
		}
		banding := 0
		if b, ok := parseNum(get(rec, "SHMI_BANDING")); ok {
			banding = int(b + 0.5)
		}
		out = append(out, TrustSHMI{
			ProviderCode:     rec[cProv],
			ProviderName:     get(rec, "PROVIDER_NAME"),
			TimePeriod:       get(rec, "TIME_PERIOD"),
			PublicationMonth: get(rec, "PUBLICATION_MONTH"),
			SHMI:             shmi,
			ODUpper:          odu,
			ODLower:          parseNumOrNaN(get(rec, "OD_LL")),
			Banding:          banding,
			Observed:         parseNumOrNaN(get(rec, "OBSERVED")),
			Expected:         parseNumOrNaN(get(rec, "EXPECTED")),
		})
	}
	return out, nil
}
