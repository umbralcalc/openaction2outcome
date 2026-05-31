// Command openaction2outcome is the CLI for the causal yardstick. It is an
// offline tool you run by hand — no server, no daemon.
//
//	fetch     Download the frozen open-data inputs into the local cache.
//	build     Mint a series' marks from the cached inputs (estimate + validate).
//	validate  Check every mark against the schema and point-in-time invariants.
//	score     Score a submission against the marks (decision + calibration).
//	study     Run the calibration study (plug-in vs model-averaged coverage).
//	export    Assemble a Hugging Face-ready dataset directory from the marks.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/umbralcalc/openaction2outcome/internal/dossier"
	"github.com/umbralcalc/openaction2outcome/internal/hfexport"
	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/internal/sbi"
	"github.com/umbralcalc/openaction2outcome/internal/series"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
	"github.com/umbralcalc/openaction2outcome/pkg/score"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "validate":
		err = cmdValidate(os.Args[2:])
	case "score":
		err = cmdScore(os.Args[2:])
	case "build":
		err = cmdBuild(os.Args[2:])
	case "fetch":
		err = cmdFetch(os.Args[2:])
	case "study":
		err = cmdStudy(os.Args[2:])
	case "export":
		err = cmdExport(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `openaction2outcome — an open causal yardstick (offline mint + scorer)

usage:
  openaction2outcome fetch [--raw DIR] [--cache DIR] [--publish-config FILE]
  openaction2outcome study [--problems N] [--seed N] [--out FILE]
  openaction2outcome build --series NAME [--raw DIR] [--cache DIR] [--dist DIR] [--marks DIR]
  openaction2outcome validate [--marks DIR]
  openaction2outcome score --submission FILE [--marks DIR] [--out FILE]
  openaction2outcome export [--marks DIR] [--card FILE] [--out DIR]
`)
}

// loadSources reads every data/raw/<id>/SOURCE.json pointer.
func loadSources(rawDir string) ([]ingest.Source, error) {
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		return nil, err
	}
	var srcs []ingest.Source
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(rawDir, e.Name(), "SOURCE.json")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		s, err := ingest.LoadSource(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		srcs = append(srcs, s)
	}
	return srcs, nil
}

// checkSourceKeys asserts every source's r2_object_key matches the layout that
// `rclone copy data/cache -> <raw_prefix>` actually produces, i.e.
// "<raw_prefix>/<source_id>/<local_path>". A mismatch means the published mirror
// URL would 404 even though the upload succeeded.
func checkSourceKeys(srcs []ingest.Source, cfg publish.Config) error {
	for _, s := range srcs {
		want := cfg.RawPrefix + "/" + s.SourceID + "/" + s.LocalPath
		if s.R2ObjectKey != want {
			return fmt.Errorf("source %q: r2_object_key %q must be %q to match the cache/upload layout",
				s.SourceID, s.R2ObjectKey, want)
		}
	}
	return nil
}

func loadPublishConfig(path string) (publish.Config, error) {
	cfg, err := publish.LoadConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return publish.Config{MarksPrefix: "marks", RawPrefix: "raw"}, nil
		}
		return cfg, err
	}
	return cfg, nil
}

// cmdStudy runs the headline calibration study: across synthetic RDD
// problems with known truth, the plug-in's sampling-only intervals under-cover
// while the SBI hybrid-BMA intervals track nominal coverage. Deterministic.
func cmdStudy(args []string) error {
	fs := flag.NewFlagSet("study", flag.ExitOnError)
	problems := fs.Int("problems", 100, "number of synthetic problems")
	n := fs.Int("n", 400, "units per problem")
	seed := fs.Int64("seed", 100, "base RNG seed")
	out := fs.String("out", "scores/calibration-study.json", "output path")
	particles := fs.Int("particles", 1200, "SMC particles per spec")
	rounds := fs.Int("rounds", 5, "SMC rounds per spec")
	fs.Parse(args)

	fmt.Printf("running calibration study: %d problems x %d specs (this takes a minute)...\n",
		*problems, len(sbi.DefaultFloorSpecs()))
	study := sbi.RunCalibrationStudy(*problems, *n, 0.3, 0.3, sbi.DefaultFloorSpecs(), *seed,
		sbi.SMCConfig{NumParticles: *particles, NumRounds: *rounds, Seed: 1})

	b, err := json.MarshalIndent(study, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("\nnominal  plug-in-cov  sbi-cov   plug-w   sbi-w\n")
	for i, L := range study.Levels {
		fmt.Printf("%5.2f      %.3f       %.3f    %.3f    %.3f\n",
			L, study.PlugIn.Coverage[i], study.SBI.Coverage[i],
			study.PlugIn.MeanWidth[i], study.SBI.MeanWidth[i])
	}
	fmt.Printf("\nwrote %s\n", *out)
	return nil
}

// cmdExport assembles a Hugging Face-ready dataset directory (per-series JSONL +
// Dataset Card) from the minted marks, staged for `huggingface-cli upload`.
func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	marksDir := fs.String("marks", "marks", "directory of mark JSON files")
	cardPath := fs.String("card", "huggingface/README.md", "Dataset Card to ship as README.md")
	out := fs.String("out", "dist/hf", "output directory (push this to Hugging Face)")
	fs.Parse(args)

	marks, err := loadMarks(*marksDir)
	if err != nil {
		return err
	}
	if len(marks) == 0 {
		return fmt.Errorf("export: no marks under %s", *marksDir)
	}
	if err := hfexport.Export(marks, *out, *cardPath); err != nil {
		return err
	}
	fmt.Printf("exported %d mark(s) -> %s (README.md + per-series .jsonl)\n", len(marks), *out)
	fmt.Printf("push with: huggingface-cli upload <user>/openaction2outcome %s . --repo-type dataset\n", *out)
	return nil
}

func cmdFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	rawDir := fs.String("raw", "data/raw", "directory of SOURCE.json pointers")
	cacheDir := fs.String("cache", "data/cache", "directory to download bytes into")
	cfgPath := fs.String("publish-config", "publish.json", "publish config (object-store mirror base)")
	fs.Parse(args)

	cfg, err := loadPublishConfig(*cfgPath)
	if err != nil {
		return err
	}
	srcs, err := loadSources(*rawDir)
	if err != nil {
		return err
	}
	if err := checkSourceKeys(srcs, cfg); err != nil {
		return err
	}
	for _, s := range srcs {
		if err := ingest.Fetch(s, *cacheDir, cfg.BaseURL); err != nil {
			return err
		}
		fmt.Printf("ok   %s -> %s\n", s.SourceID, s.CachePath(*cacheDir))
	}
	fmt.Printf("fetched %d source(s)\n", len(srcs))
	return nil
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	marksDir := fs.String("marks", "marks", "directory of mark JSON files")
	fs.Parse(args)

	paths, err := markPaths(*marksDir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		fmt.Printf("no marks found under %s\n", *marksDir)
		return nil
	}
	for _, p := range paths {
		m, err := schema.LoadMark(p)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		fmt.Printf("ok   %s  (%s, %s, admitted=%v)\n", filepath.Base(p), m.Series, m.RDDType, m.Dossier.Admitted)
	}
	fmt.Printf("validated %d mark(s)\n", len(paths))
	return nil
}

func cmdScore(args []string) error {
	fs := flag.NewFlagSet("score", flag.ExitOnError)
	marksDir := fs.String("marks", "marks", "directory of mark JSON files")
	subPath := fs.String("submission", "", "submission JSON file (required)")
	outPath := fs.String("out", "", "write full JSON report to this file (optional)")
	fs.Parse(args)

	if *subPath == "" {
		return fmt.Errorf("--submission is required")
	}
	marks, err := loadMarks(*marksDir)
	if err != nil {
		return err
	}
	sub, err := schema.LoadSubmission(*subPath)
	if err != nil {
		return err
	}
	rep := score.ScoreSubmission(marks, sub, score.Options{})
	fmt.Print(rep.String())

	if *outPath != "" {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*outPath, append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote report to %s\n", *outPath)
	}
	return nil
}

func cmdBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	seriesName := fs.String("series", "", "series to mint: floor-standards")
	rawDir := fs.String("raw", "data/raw", "directory of SOURCE.json pointers")
	cacheDir := fs.String("cache", "data/cache", "directory of fetched input bytes")
	distDir := fs.String("dist", "dist", "directory to stage published artifacts (episode sidecars)")
	outDir := fs.String("marks", "marks", "directory to write the minted mark")
	dossierDir := fs.String("dossiers", "dossiers", "directory to write the rendered validity dossier")
	cfgPath := fs.String("publish-config", "publish.json", "publish config (object-store base URL)")
	noFetch := fs.Bool("offline", false, "do not fetch; require inputs already cached")
	fs.Parse(args)

	cfg, err := loadPublishConfig(*cfgPath)
	if err != nil {
		return err
	}
	if !*noFetch {
		srcs, err := loadSources(*rawDir)
		if err != nil {
			return err
		}
		if err := checkSourceKeys(srcs, cfg); err != nil {
			return err
		}
		for _, s := range srcs {
			if err := ingest.Fetch(s, *cacheDir, cfg.BaseURL); err != nil {
				return err
			}
		}
	}

	var mark schema.Mark
	switch *seriesName {
	case "floor-standards":
		mark, err = series.BuildFloorStandards(*rawDir, *cacheDir, *distDir, cfg)
	case "shmi":
		mark, err = series.BuildSHMI(*rawDir, *cacheDir, *distDir, cfg)
	case "shmi-fuzzy":
		mark, err = series.BuildSHMIFuzzy(*rawDir, *cacheDir, *distDir, cfg)
	case "bathing-water":
		mark, err = series.BuildBathingWater(*rawDir, *cacheDir, *distDir, cfg)
	case "":
		return fmt.Errorf("build: --series is required (floor-standards, shmi, shmi-fuzzy, bathing-water)")
	default:
		return fmt.Errorf("build: unknown or not-yet-implemented series %q", *seriesName)
	}
	if err != nil {
		return err
	}

	// A mark only joins the published corpus if it passes the validity battery
	// (and, for fuzzy designs, a strong first stage). A failing candidate is
	// reported but not written, so the dataset holds only trustworthy references.
	if !mark.Dossier.Admitted {
		fmt.Printf("NOT ADMITTED: %s (%s) — not written to %s\n", mark.ID, mark.Series, *outDir)
		fmt.Printf("  %s\n", mark.Dossier.Notes)
		return nil
	}

	out := filepath.Join(*outDir, mark.ID+".json")
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := schema.WriteMark(f, mark); err != nil {
		return err
	}
	dossierPath := filepath.Join(*dossierDir, mark.ID+".md")
	if err := os.WriteFile(dossierPath, []byte(dossier.Render(mark)), 0o644); err != nil {
		return err
	}
	fmt.Printf("minted %s (%s, admitted=%v, effect=%.4g [%.4g, %.4g], episodes=%d)\n",
		out, mark.Series, mark.Dossier.Admitted, mark.Effect.Central,
		mark.Effect.Interval.Lower, mark.Effect.Interval.Upper, mark.Data.Rows)
	fmt.Printf("staged  %s  (sha256=%s, %d bytes)\n", mark.Data.URI, mark.Data.SHA256, mark.Data.Bytes)
	return nil
}

func loadMarks(dir string) ([]schema.Mark, error) {
	paths, err := markPaths(dir)
	if err != nil {
		return nil, err
	}
	marks := make([]schema.Mark, 0, len(paths))
	for _, p := range paths {
		m, err := schema.LoadMark(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		marks = append(marks, m)
	}
	return marks, nil
}

// markPaths returns the sorted *.json files under dir (non-recursive).
func markPaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}
