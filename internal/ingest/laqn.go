package ingest

import (
	"fmt"
	"math"
	"strings"
)

// LAQNStationMonth is one (monitoring station × month) record from the frozen LAQN
// NO2 harvest (scripts/ulez_harvest.py). Each row carries the station's monthly-mean
// NO2 with the number of valid hours behind it (the capture count, used to drop
// thinly-sampled months), its group (treated outer-London roadside/kerbside vs
// control urban-background/suburban), and the fixed station metadata.
type LAQNStationMonth struct {
	SiteCode  string
	SiteName  string
	LAName    string
	SiteType  string
	Latitude  float64
	Longitude float64
	Group     string // "treated" | "control"
	Month     string // "YYYY-MM"
	NO2Mean   float64
	NHours    int
}

// LoadLAQNNO2 reads the frozen monthly NO2 panel.
func LoadLAQNNO2(csvPath string) ([]LAQNStationMonth, error) {
	rows, col, err := readCSV(csvPath)
	if err != nil {
		return nil, err
	}
	for _, c := range []string{"site_code", "group", "month", "no2_mean", "n_hours"} {
		if _, ok := col[c]; !ok {
			return nil, fmt.Errorf("laqn no2: missing expected column %q", c)
		}
	}
	get := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}
	out := make([]LAQNStationMonth, 0, len(rows))
	for _, rec := range rows {
		v := parseNumOrNaN(get(rec, "no2_mean"))
		if math.IsNaN(v) {
			continue
		}
		nh := 0
		if n, ok := parseNum(get(rec, "n_hours")); ok {
			nh = int(n + 0.5)
		}
		out = append(out, LAQNStationMonth{
			SiteCode:  get(rec, "site_code"),
			SiteName:  get(rec, "site_name"),
			LAName:    get(rec, "la_name"),
			SiteType:  get(rec, "site_type"),
			Latitude:  parseNumOrNaN(get(rec, "latitude")),
			Longitude: parseNumOrNaN(get(rec, "longitude")),
			Group:     strings.ToLower(strings.TrimSpace(get(rec, "group"))),
			Month:     strings.TrimSpace(get(rec, "month")),
			NO2Mean:   v,
			NHours:    nh,
		})
	}
	return out, nil
}

// LAQNMeteoMonth is one month's meteorology (the dominant short-window NO2
// confounder) from the frozen Open-Meteo ERA5 archive harvest: a central-London
// monthly-mean temperature, wind speed, direction-resolved wind components, and
// total precipitation. Wind is stored as u/v components so a monthly mean respects
// direction; speed is the scalar mean.
type LAQNMeteoMonth struct {
	Month        string
	TempC        float64
	WindSpeedKmh float64
	WindUKmh     float64
	WindVKmh     float64
	PrecipMm     float64
}

// LoadLAQNMeteo reads the frozen monthly meteorology panel.
func LoadLAQNMeteo(csvPath string) (map[string]LAQNMeteoMonth, error) {
	rows, col, err := readCSV(csvPath)
	if err != nil {
		return nil, err
	}
	for _, c := range []string{"month", "temp_c", "wind_speed_kmh"} {
		if _, ok := col[c]; !ok {
			return nil, fmt.Errorf("laqn meteo: missing expected column %q", c)
		}
	}
	get := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}
	out := make(map[string]LAQNMeteoMonth, len(rows))
	for _, rec := range rows {
		m := strings.TrimSpace(get(rec, "month"))
		if m == "" {
			continue
		}
		out[m] = LAQNMeteoMonth{
			Month:        m,
			TempC:        parseNumOrNaN(get(rec, "temp_c")),
			WindSpeedKmh: parseNumOrNaN(get(rec, "wind_speed_kmh")),
			WindUKmh:     parseNumOrNaN(get(rec, "wind_u_kmh")),
			WindVKmh:     parseNumOrNaN(get(rec, "wind_v_kmh")),
			PrecipMm:     parseNumOrNaN(get(rec, "precip_mm")),
		}
	}
	return out, nil
}
