package schema

import (
	"math"
	"testing"
)

func TestDistributionValidate(t *testing.T) {
	ok := Distribution{Central: 0.5, Interval: &Interval{Level: 0.95, Lower: 0.3, Upper: 0.7}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid distribution rejected: %v", err)
	}

	cases := map[string]Distribution{
		"level >= 1":         {Central: 0.5, Interval: &Interval{Level: 1.5, Lower: 0.3, Upper: 0.7}},
		"level <= 0":         {Central: 0.5, Interval: &Interval{Level: 0, Lower: 0.3, Upper: 0.7}},
		"lower > upper":      {Central: 0.5, Interval: &Interval{Level: 0.95, Lower: 0.8, Upper: 0.2}},
		"central below":      {Central: 0.1, Interval: &Interval{Level: 0.95, Lower: 0.3, Upper: 0.7}},
		"central above":      {Central: 0.9, Interval: &Interval{Level: 0.95, Lower: 0.3, Upper: 0.7}},
		"negative std":       {Central: 0.5, StdDev: ptr(-1)},
		"unsorted quantiles": {Central: 0.5, Quantiles: []Quantile{{P: 0.9, Value: 1}, {P: 0.1, Value: 0}}},
		"quantile p>1":       {Central: 0.5, Quantiles: []Quantile{{P: 1.2, Value: 1}}},
	}
	for name, d := range cases {
		if d.Validate() == nil {
			t.Errorf("expected %q to be rejected", name)
		}
	}
}

func TestQuantileAtFromQuantiles(t *testing.T) {
	d := Distribution{Central: 0, Quantiles: []Quantile{{0.0, -1}, {0.5, 0}, {1.0, 1}}}
	got, ok := d.QuantileAt(0.25)
	if !ok {
		t.Fatal("expected ok from quantiles")
	}
	if math.Abs(got-(-0.5)) > 1e-9 { // linear interpolation between (0,-1) and (0.5,0)
		t.Errorf("interpolated quantile: got %v want -0.5", got)
	}
	// Clamping below/above the supplied range.
	if v, _ := d.QuantileAt(-1); v != -1 {
		t.Errorf("clamp low: %v", v)
	}
	if v, _ := d.QuantileAt(2); v != 1 {
		t.Errorf("clamp high: %v", v)
	}
}

func TestQuantileAtFromSamples(t *testing.T) {
	d := Distribution{Central: 0, Samples: []float64{4, 1, 3, 2}} // unsorted on purpose
	med, ok := d.QuantileAt(0.5)
	if !ok {
		t.Fatal("expected ok from samples")
	}
	if med < 2 || med > 3 {
		t.Errorf("sample median should be in [2,3], got %v", med)
	}
}

func TestQuantileAtUnavailable(t *testing.T) {
	d := Distribution{Central: 0, Interval: &Interval{Level: 0.95, Lower: -1, Upper: 1}}
	if _, ok := d.QuantileAt(0.5); ok {
		t.Fatal("QuantileAt should report false when only an interval is present")
	}
}

func ptr(f float64) *float64 { return &f }
