package schema

import (
	"errors"
	"fmt"
)

// Provenance records where a mark's inputs came from, under what licence, the
// point-in-time timestamps that defend against leakage, and the determinism
// record that makes the mark re-mintable byte-for-byte.
type Provenance struct {
	// Sources are the frozen open-data inputs, each with its own licence + hash.
	Sources []Source `json:"sources"`

	// Point-in-time timestamps. The invariant is:
	//     ContextAsOf <= DecisionTimestamp < OutcomeTimestamp
	// All are ISO-8601 dates/datetimes. OutcomeTimestamp may be empty for marks
	// whose outcome horizon has not yet been reached (such a mark is structurally
	// valid but carries no realized outcome and must be flagged as such).
	ContextAsOf       string `json:"context_asof"`
	DecisionTimestamp string `json:"decision_timestamp"`
	OutcomeTimestamp  string `json:"outcome_timestamp,omitempty"`

	// RunningVariableVintage pins the running variable to the exact vintage used
	// at decision time (critical for periodically-revised indices like IMD/SHMI).
	RunningVariableVintage string `json:"running_variable_vintage"`

	// FundingRound (area-funding series) or analogous decision-round identifier.
	DecisionRound string `json:"decision_round,omitempty"`

	// Determinism record.
	Seed         *int64            `json:"seed,omitempty"`
	ToolVersions map[string]string `json:"tool_versions,omitempty"`

	// OutcomeRealized is false when OutcomeTimestamp lies in the future relative
	// to mint time — i.e. the mark exercises the design on real running-variable
	// and treatment data but its downstream outcome has not yet occurred.
	OutcomeRealized bool `json:"outcome_realized"`
}

// Source is one frozen open-data input.
type Source struct {
	SourceID    string `json:"source_id"`
	Title       string `json:"title"`
	Publisher   string `json:"publisher"`
	DownloadURI string `json:"download_uri"`
	LandingPage string `json:"landing_page,omitempty"`
	RetrievedAt string `json:"retrieved_at"`
	Licence     string `json:"licence"`
	SHA256      string `json:"sha256"`
	Vintage     string `json:"vintage,omitempty"`
}

// Validate checks provenance invariants that any consumer can rely on: the
// point-in-time ordering and the presence of licence + hash on every source.
//
// Timestamps are compared lexically, which is correct for zero-padded ISO-8601
// dates/datetimes in a single timezone — sufficient for the date-granular
// pinning the marks use. (A future revision may parse to time.Time if
// sub-day or cross-timezone precision is ever needed.)
func (p Provenance) Validate() error {
	if len(p.Sources) == 0 {
		return errors.New("provenance: no sources recorded")
	}
	for i, s := range p.Sources {
		if s.SourceID == "" {
			return fmt.Errorf("provenance: source %d has empty source_id", i)
		}
		if s.Licence == "" {
			return fmt.Errorf("provenance: source %q has empty licence", s.SourceID)
		}
		if s.SHA256 == "" {
			return fmt.Errorf("provenance: source %q has empty sha256", s.SourceID)
		}
	}
	if p.ContextAsOf == "" || p.DecisionTimestamp == "" {
		return errors.New("provenance: context_asof and decision_timestamp are required")
	}
	if p.ContextAsOf > p.DecisionTimestamp {
		return fmt.Errorf("provenance: context_asof %q must be <= decision_timestamp %q",
			p.ContextAsOf, p.DecisionTimestamp)
	}
	if p.OutcomeTimestamp != "" {
		if p.DecisionTimestamp >= p.OutcomeTimestamp {
			return fmt.Errorf("provenance: decision_timestamp %q must be < outcome_timestamp %q",
				p.DecisionTimestamp, p.OutcomeTimestamp)
		}
	} else if p.OutcomeRealized {
		return errors.New("provenance: outcome_realized is true but outcome_timestamp is empty")
	}
	if p.RunningVariableVintage == "" {
		return errors.New("provenance: running_variable_vintage is required (leakage guard)")
	}
	return nil
}
