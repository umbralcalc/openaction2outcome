// Command openaction2outcome is the CLI for the causal yardstick. It is an
// offline tool you run by hand — no server, no daemon.
//
//	fetch     Download the frozen open-data inputs into the local cache.
//	build     Mint a series' marks from the cached inputs (estimate + validate).
//	validate  Check every mark against the schema and point-in-time invariants.
//	score     Score a submission against the marks (decision + calibration).
//	study     Run the calibration study (plug-in vs model-averaged coverage).
//	export    Assemble a Hugging Face-ready dataset directory from the marks.
//	site      Generate the static GitHub Pages site (docs/) from the marks + docs.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/umbralcalc/openaction2outcome/internal/bridge"
	"github.com/umbralcalc/openaction2outcome/internal/dossier"
	"github.com/umbralcalc/openaction2outcome/internal/episodes"
	"github.com/umbralcalc/openaction2outcome/internal/hfexport"
	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/internal/sbi"
	"github.com/umbralcalc/openaction2outcome/internal/series"
	"github.com/umbralcalc/openaction2outcome/internal/site"
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
	case "site":
		err = cmdSite(os.Args[2:])
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
  openaction2outcome study [--problems N] [--seed N] [--out FILE] [--bridge [--compare|--layer]]
  openaction2outcome build --series NAME [--raw DIR] [--cache DIR] [--dist DIR] [--marks DIR]
  openaction2outcome validate [--marks DIR]
  openaction2outcome score --submission FILE [--marks DIR] [--out FILE]
  openaction2outcome export [--marks DIR] [--card FILE] [--out DIR]
  openaction2outcome site [--out DIR] [--repo-url URL] [--hf-repo REPO]
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
	isBridge := fs.Bool("bridge", false, "run the bridge recovery + LOAO study instead of the RDD calibration study")
	compare := fs.Bool("compare", false, "with --bridge: compare modular vs exact-joint vs sampled-joint calibrators")
	layer := fs.Bool("layer", false, "with --bridge: run the deterministic causal-layer study (moment vs SMC, gate, re-mint)")
	fs.Parse(args)

	if *isBridge {
		switch {
		case *layer:
			return runDeterministicLayerStudy(*problems, *seed, *particles, *rounds, *out)
		case *compare:
			return runBridgeComparison(*problems, *seed, *particles, *rounds, *out)
		default:
			return runBridgeStudy(*problems, *seed, *particles, *rounds, *out)
		}
	}

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

// runBridgeStudy runs the bridge machinery-validation study: across synthetic
// mechanisms with a KNOWN true effect curve, the pinned GP-discrepancy posterior
// recovers the truth between anchors and held-out anchors fall within the
// predicted interval (LOAO). Reported separately from the RDD calibration study,
// never pooled (the cardinal pin/span rule).
func runBridgeStudy(problems int, seed int64, particles, rounds int, out string) error {
	fmt.Printf("running bridge recovery study: %d synthetic problems (this takes a moment)...\n", problems)
	study := bridge.RunBridgeRecoveryStudy(problems, seed,
		bridge.SMCConfig{NumParticles: particles, NumRounds: rounds, Seed: 1})

	b, err := json.MarshalIndent(study, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("\nkernel=%s\nnominal  recovery-cov  loao-cov   recovery-w\n", study.Kernel)
	for i, L := range study.Levels {
		fmt.Printf("%5.2f      %.3f         %.3f      %.3f\n",
			L, study.Recovery.Coverage[i], study.LOAO.Coverage[i], study.Recovery.MeanWidth[i])
	}
	fmt.Printf("\nwrote %s\n", out)
	return nil
}

// runDeterministicLayerStudy runs the deterministic causal-layer validation: on a
// directed structural causal mechanism (do(T=x) on a confounded graph, run on the
// stochadex engine) with a KNOWN interventional truth, it shows the moment
// calibrator matches the SMC closed-form joint while carrying no sampling noise,
// re-mints byte-for-byte, earns the closed-form rung from the tractability gate,
// and recovers the truth between anchors.
func runDeterministicLayerStudy(problems int, seed int64, particles, rounds int, out string) error {
	fmt.Printf("running deterministic causal-layer study: %d problems on the structural causal mechanism...\n", problems)
	study := bridge.RunDeterministicLayerStudy(problems, seed,
		bridge.SMCConfig{NumParticles: particles, NumRounds: rounds, Seed: 1})

	b, err := json.MarshalIndent(study, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("\nmechanism=%s  kernel=%s\n", study.Mechanism, study.Kernel)
	fmt.Printf("moment vs SMC closed-form: max=%.5f mean=%.5f   re-mint identical=%v\n",
		study.MaxMomentVsSMC, study.MeanMomentVsSMC, study.RemintIdentical)
	fmt.Printf("tractability gate: all closed-form=%v (rung=%s)\n", study.AllClosedForm, study.GateRung)
	fmt.Printf("\nnominal  recovery-cov  recovery-w\n")
	for i, L := range study.Levels {
		fmt.Printf("%5.2f      %.3f         %.3f\n", L, study.Recovery.Coverage[i], study.Recovery.MeanWidth[i])
	}
	fmt.Printf("\n%s\n\nwrote %s\n", study.Finding, out)
	return nil
}

// runBridgeComparison runs the three calibrators (modular cut, exact closed-form
// joint, stochadex-sampled joint) on identical synthetic problems and reports how
// each recovers the known truth — the empirical answer to "why condition the GP
// discrepancy in closed form rather than sample it through stochadex".
func runBridgeComparison(problems int, seed int64, particles, rounds int, out string) error {
	fmt.Printf("running bridge method comparison: %d problems x 3 calibrators...\n", problems)
	cmp := bridge.RunBridgeComparison(problems, seed,
		bridge.SMCConfig{NumParticles: particles, NumRounds: rounds, Seed: 1})

	b, err := json.MarshalIndent(cmp, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("\nkernel=%s  (recovery coverage of the KNOWN truth; nominal levels %v)\n", cmp.Kernel, cmp.Levels)
	fmt.Printf("%-30s %s\n", "method", "coverage  /  mean width")
	for _, m := range cmp.Methods {
		fmt.Printf("%-30s %v\n", m.Method, fmtCov(m.Coverage))
		fmt.Printf("%-30s %v\n", "", fmtCov(m.MeanWidth))
	}
	fmt.Printf("\n%s\n\nwrote %s\n", cmp.Finding, out)
	return nil
}

func fmtCov(xs []float64) string {
	s := ""
	for _, x := range xs {
		s += fmt.Sprintf("%6.3f ", x)
	}
	return s
}

// cmdExport assembles a Hugging Face-ready dataset directory (per-series JSONL +
// Dataset Card) from the minted marks, staged for `huggingface-cli upload`. It
// writes the per-mark episodes manifest (the object-storage dataset is one gzipped
// CSV per mark, listed with its sha256), and mirrors those same per-mark CSVs into
// the Hugging Face directory so an HF user can load one mark's rows directly.
func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	marksDir := fs.String("marks", "marks", "directory of mark JSON files")
	cardPath := fs.String("card", "huggingface/README.md", "Dataset Card to ship as README.md")
	out := fs.String("out", "dist/hf", "output directory (push this to Hugging Face)")
	distDir := fs.String("dist", "dist", "directory holding the staged per-mark episode tables")
	manifestPath := fs.String("manifest", "datasets/episodes.manifest.json", "git-tracked pointer to the published episodes dataset")
	cfgPath := fs.String("publish-config", "publish.json", "publish config (object-store base URL)")
	fs.Parse(args)

	cfg, err := loadPublishConfig(*cfgPath)
	if err != nil {
		return err
	}
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

	// Write the per-mark episodes manifest (the object-storage dataset is one
	// gzipped CSV per mark; the manifest lists each file + its hash).
	mf, err := episodes.NewManifest(marks, *distDir, cfg)
	if err != nil {
		return err
	}
	if err := episodes.WriteManifest(*manifestPath, mf); err != nil {
		return err
	}
	fmt.Printf("wrote manifest %s (%d marks, %d rows total)\n", *manifestPath, len(mf.Marks), mf.TotalRows)

	// Mirror the same per-mark CSVs into the Hugging Face dataset dir (same
	// schema as object storage — no unioned re-encoding) so an HF user can pull a
	// mark's rows directly.
	written, err := episodes.CopyToHF(marks, *distDir, *out)
	if err != nil {
		return err
	}
	fmt.Printf("mirrored %d per-mark episodes.csv.gz -> %s/episodes/\n", len(written), *out)
	fmt.Printf("push with: huggingface-cli upload <user>/openaction2outcome %s . --repo-type dataset\n", *out)
	return nil
}

// cmdSite generates the static GitHub Pages site into docs/ from the committed
// marks, dossiers, schema docs, and dataset manifest. It is a faithful, offline,
// deterministic HTML view of artifacts already in the repo — re-run it after a
// mint so the site tracks the data. Pages serves the committed docs/ folder.
func cmdSite(args []string) error {
	fs := flag.NewFlagSet("site", flag.ExitOnError)
	cfg := site.Config{}
	fs.StringVar(&cfg.MarksDir, "marks", "marks", "directory of mark JSON files")
	fs.StringVar(&cfg.DossiersDir, "dossiers", "dossiers", "directory of rendered dossiers")
	fs.StringVar(&cfg.SchemaDoc, "schema-doc", "docs/schema.md", "data-dictionary markdown")
	fs.StringVar(&cfg.ChangelogDoc, "changelog-doc", "CHANGELOG.md", "changelog markdown")
	fs.StringVar(&cfg.StudyPath, "study", "scores/calibration-study.json", "calibration study (for the headline finding)")
	fs.StringVar(&cfg.ManifestPath, "manifest", "datasets/episodes.manifest.json", "episodes dataset manifest")
	fs.StringVar(&cfg.RawDir, "raw", "data/raw", "directory of SOURCE.json pointers")
	fs.StringVar(&cfg.PublishConfig, "publish-config", "publish.json", "publish config (object-store base URL)")
	fs.StringVar(&cfg.LogoPath, "logo", "docs/assets/logo.png", "logo to copy into the site")
	fs.StringVar(&cfg.OutDir, "out", "docs", "output directory (the Pages /docs folder)")
	fs.StringVar(&cfg.RepoURL, "repo-url", "https://github.com/umbralcalc/openaction2outcome", "GitHub repo base URL")
	fs.StringVar(&cfg.HFRepo, "hf-repo", "umbralcalc/openaction2outcome", "Hugging Face dataset repo")
	fs.Parse(args)

	if err := site.Generate(cfg); err != nil {
		return err
	}
	fmt.Printf("generated site -> %s (open %s/index.html)\n", cfg.OutDir, cfg.OutDir)
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
		design := string(m.EffectiveIdentification())
		admitted := m.Dossier.Admitted
		if m.EffectiveCategory() == schema.CategoryBridge {
			design = "bridge"
			if m.Dossier.Bridge != nil {
				admitted = m.Dossier.Bridge.Admitted
			}
		}
		fmt.Printf("ok   %s  (%s, %s, %s, admitted=%v)\n", filepath.Base(p), m.EffectiveCategory(), m.Series, design, admitted)
	}
	fmt.Printf("validated %d mark(s)\n", len(paths))
	return nil
}

func cmdScore(args []string) error {
	fs := flag.NewFlagSet("score", flag.ExitOnError)
	marksDir := fs.String("marks", "marks", "directory of mark JSON files")
	subPath := fs.String("submission", "", "submission JSON file (required)")
	outPath := fs.String("out", "", "write full JSON report to this file (optional)")
	category := fs.String("category", "both", "which marks to score: identified | bridge | both")
	fs.Parse(args)

	if *subPath == "" {
		return fmt.Errorf("--submission is required")
	}
	var cats []schema.Category
	switch *category {
	case "identified":
		cats = []schema.Category{schema.CategoryIdentified}
	case "bridge":
		cats = []schema.Category{schema.CategoryBridge}
	case "both", "":
		cats = nil // all categories; still reported separately, never pooled
	default:
		return fmt.Errorf("--category must be one of identified, bridge, both (got %q)", *category)
	}
	marks, err := loadMarks(*marksDir)
	if err != nil {
		return err
	}
	sub, err := schema.LoadSubmission(*subPath)
	if err != nil {
		return err
	}
	rep := score.ScoreSubmission(marks, sub, score.Options{Categories: cats})
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
	case "bathing-water":
		mark, err = series.BuildBathingWater(*rawDir, *cacheDir, *distDir, cfg)
	case "ulez-no2":
		mark, err = series.BuildULEZNO2(*rawDir, *cacheDir, *distDir, cfg)
	case "ulez-no2-2023":
		mark, err = series.BuildULEZNO22023(*rawDir, *cacheDir, *distDir, cfg)
	case "berlin-lez-no2":
		mark, err = series.BuildBerlinLEZ(*rawDir, *cacheDir, *distDir, cfg)
	case "madrid-lez-no2":
		mark, err = series.BuildMadridLEZ(*rawDir, *cacheDir, *distDir, cfg)
	case "ca-menthol-smoking":
		mark, err = series.BuildCAMenthol(*rawDir, *cacheDir, *distDir, cfg)
	case "":
		return fmt.Errorf("build: --series is required (floor-standards, shmi, bathing-water, berlin-lez-no2, madrid-lez-no2, ca-menthol-smoking)")
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
	fmt.Printf("minted %s (%s, admitted=%v, effect=%.4g [%.4g, %.4g])\n",
		out, mark.Series, mark.Dossier.Admitted, mark.Effect.Central,
		mark.Effect.Interval.Lower, mark.Effect.Interval.Upper)
	fmt.Printf("staged  %s/marks/%s/episodes.csv.gz  (build intermediate; reshaped into the episodes dataset by `export`)\n", *distDir, mark.ID)
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
