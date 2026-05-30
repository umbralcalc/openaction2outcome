package schema

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// WriteMark encodes a Mark as indented JSON. It validates before writing so an
// invalid mark never reaches disk.
func WriteMark(w io.Writer, m Mark) error {
	if err := m.Validate(); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

// LoadMark reads and validates a Mark from a JSON file.
func LoadMark(path string) (Mark, error) {
	var m Mark
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("decode mark %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return m, fmt.Errorf("validate mark %s: %w", path, err)
	}
	return m, nil
}

// LoadSubmission reads and validates a Submission from a JSON file.
func LoadSubmission(path string) (Submission, error) {
	var s Submission
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("decode submission %s: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return s, fmt.Errorf("validate submission %s: %w", path, err)
	}
	return s, nil
}
