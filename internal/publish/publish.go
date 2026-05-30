// Package publish handles the object-storage side of the offline pipeline: the
// publishing config (the public base URL of the bucket) and writing the
// per-mark analysis-ready episode table as a deterministic, content-addressed
// artifact staged for upload (DEV_PLAN §5). Nothing here talks to the network;
// uploading the staged artifacts is a separate, credentialed step.
package publish

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Config is publish.json: where published artifacts are served from.
type Config struct {
	BaseURL     string `json:"base_url"`
	Bucket      string `json:"bucket"`
	RawPrefix   string `json:"raw_prefix"`
	MarksPrefix string `json:"marks_prefix"`
}

// LoadConfig reads publish.json.
func LoadConfig(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("decode %s: %w", path, err)
	}
	if c.MarksPrefix == "" {
		c.MarksPrefix = "marks"
	}
	if c.RawPrefix == "" {
		c.RawPrefix = "raw"
	}
	return c, nil
}

// MarkObjectKey is the bucket key for a per-mark artifact.
func (c Config) MarkObjectKey(markID, name string) string {
	return c.MarksPrefix + "/" + markID + "/" + name
}

// MarkArtifactURL is the public download URL for a per-mark artifact.
func (c Config) MarkArtifactURL(markID, name string) string {
	return strings.TrimRight(c.BaseURL, "/") + "/" + c.MarkObjectKey(markID, name)
}

// WrittenArtifact describes a staged artifact: its local staging path, content
// hash, size, and row count.
type WrittenArtifact struct {
	Path   string
	SHA256 string
	Bytes  int64
	Rows   int
}

// WriteEpisodesCSVGz writes the episode table to distDir/marks/<markID>/<name>
// as deterministic gzip (no embedded name/mtime), returning its content hash and
// size so the mark can reference it. Determinism keeps re-mints byte-identical.
func WriteEpisodesCSVGz(distDir, markID, name string, header []string, rows [][]string) (WrittenArtifact, error) {
	var wa WrittenArtifact
	dir := filepath.Join(distDir, "marks", markID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return wa, err
	}
	path := filepath.Join(dir, name)

	f, err := os.Create(path)
	if err != nil {
		return wa, err
	}
	gz, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	// Leave gz.Header zero-valued (no Name, no ModTime) for deterministic output.
	cw := csv.NewWriter(gz)
	if err := cw.Write(header); err != nil {
		f.Close()
		return wa, err
	}
	if err := cw.WriteAll(rows); err != nil {
		f.Close()
		return wa, err
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		f.Close()
		return wa, err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return wa, err
	}
	if err := f.Close(); err != nil {
		return wa, err
	}

	sum, size, err := hashFile(path)
	if err != nil {
		return wa, err
	}
	return WrittenArtifact{Path: path, SHA256: sum, Bytes: size, Rows: len(rows)}, nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
