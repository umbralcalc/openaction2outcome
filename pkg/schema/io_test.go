package schema

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func validMark() Mark {
	return Mark{
		SchemaVersion: SchemaVersion,
		ID:            "demo",
		Series:        SeriesFloorStandards,
		Domain:        "Education",
		UnitType:      "school",
		RDDType:       Sharp,
		Design: Design{
			RunningVariable: Variable{Name: "score"},
			Cutoff:          -0.5,
			Direction:       BelowTreated,
			Action:          "intervene",
			Alternative:     "do nothing",
			Outcome:         Variable{Name: "score_later"},
			Estimand:        "sharp RD effect",
		},
		Effect:     Distribution{Central: 0.1, Interval: &Interval{Level: 0.95, Lower: -0.1, Upper: 0.3}},
		Dossier:    ValidityDossier{Admitted: true},
		Provenance: goodProvenance(),
	}
}

func TestMarkRoundTrip(t *testing.T) {
	m := validMark()
	var buf bytes.Buffer
	if err := WriteMark(&buf, m); err != nil {
		t.Fatalf("WriteMark: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "m.json")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMark(path)
	if err != nil {
		t.Fatalf("LoadMark: %v", err)
	}
	if got.ID != m.ID || got.Series != m.Series || got.Effect.Central != m.Effect.Central ||
		got.Design.Cutoff != m.Design.Cutoff {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, m)
	}
}

func TestWriteMarkRejectsInvalid(t *testing.T) {
	m := validMark()
	m.Effect.Interval = nil // a mark must carry an interval
	if err := WriteMark(&bytes.Buffer{}, m); err == nil {
		t.Fatal("WriteMark should refuse an invalid mark")
	}
}

func TestLoadMarkErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{not json"), 0o644)
	if _, err := LoadMark(bad); err == nil {
		t.Fatal("expected decode error")
	}
	if _, err := LoadMark(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("expected read error for missing file")
	}
}

func TestMarkValidateEdgeCases(t *testing.T) {
	base := validMark()

	bad := base
	bad.SchemaVersion = "0.0.1"
	mustReject(t, bad, "wrong schema version")

	bad = base
	bad.Series = "nonsense"
	mustReject(t, bad, "unknown series")

	bad = base
	bad.Design.Direction = "sideways"
	mustReject(t, bad, "unknown direction")

	bad = base
	bad.RDDType = Fuzzy // fuzzy needs a first stage
	mustReject(t, bad, "fuzzy without first stage")

	good := base
	good.RDDType = Fuzzy
	good.Dossier.FirstStage = &FirstStageResult{Jump: 0.4, Passed: true}
	if err := good.Validate(); err != nil {
		t.Errorf("fuzzy mark with a first stage should validate: %v", err)
	}
}

func mustReject(t *testing.T, m Mark, why string) {
	t.Helper()
	if err := m.Validate(); err == nil {
		t.Errorf("expected rejection: %s", why)
	}
}
