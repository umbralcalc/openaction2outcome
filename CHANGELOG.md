# Changelog

All notable changes to this project are recorded here. Versions refer to the
published dataset + tooling release (the wire-format `schema_version` is tracked
separately inside each mark).

## Unreleased

### Marks (admitted reference points)
- **bathing-water-poor-2015** — sharp RDD on the English bathing-water Poor/Sufficient
  classification boundary (revised Bathing Water Directive). Running variable is the
  base-10 log compliance margin (worst of the E. coli / intestinal enterococci
  90th-percentile statistic over its Sufficient threshold, coastal vs inland);
  crossing into Poor triggers a mandatory advice-against-bathing sign + EA catchment
  investigation. Outcome is the same site's compliance margin four years later
  (2015 → 2019, non-overlapping rolling sample windows). Effect −0.095, 95% interval
  [−0.407, 0.245] (the bandwidth sweep flips sign — genuine identification
  uncertainty, honestly reported).

### Schema
- New `bathing-water` series value, and an optional `seam_specific_checks` field on
  the validity dossier for series-specific tests outside the standard battery.

### New validity check
- **Abnormal-sample-exclusion sensitivity** (the bathing-water manipulation analogue):
  re-includes the discretionarily-discounted extreme-weather samples, re-derives the
  log-normal 90th-percentile margin near the cutoff, and confirms the effect estimate
  is robust. The mark records how many samples were discounted near the boundary and
  how many sites flip Poor/Sufficient under re-inclusion.

### Data
- Bathing-water inputs are harvested from the Defra linked-data API (no bulk CSV
  upstream) by `scripts/bathingwater_harvest.py` into two frozen, hash-pinned CSVs
  (annual compliance + percentile statistics; per-sample microbiology for near-cutoff
  windows), mirroring the existing derived-snapshot pattern.

## v0.1.0 — 2026-05-30

First public release: a working causal yardstick on two domains, with both
identification regimes implemented and the headline calibration finding
demonstrated.

### Marks (admitted reference points)
- **floor-standards-p8-2016** — sharp RDD on the 2016 English school Progress 8
  floor (−0.5); outcome is each school's Progress 8 two years later. Effect
  0.028, 95% interval [−0.054, 0.256].
- **shmi-higher-than-expected-banding** — sharp intention-to-treat RDD on the NHS
  SHMI "higher than expected" mortality banding; outcome is the trust's SHMI in
  the next 12-month window (pooled trust-years). Effect −0.013, 95% interval
  [−0.066, 0.018].

### Estimation
- Model-averaged SBI estimator (stochadex SMC across a bandwidth × order × kernel
  grid; marginal-likelihood weighting within a bandwidth, uniform across). The
  honest interval splits into sampling vs identification variance.
- Fuzzy two-stage (Wald LATE) estimator with a first-stage admission gate.
- Plug-in local-linear estimator kept as the comparison baseline.

### Headline finding
- Calibration study against known truth: at a nominal 95% interval the plug-in
  method covers the truth 80% of the time, the model-averaged method 92%.

### Tooling & distribution
- CLI: `fetch`, `build`, `validate`, `score`, `study`, `export`.
- Two public, dependency-light Go packages: `pkg/schema` and `pkg/score`.
- Deterministic, re-mintable marks; per-mark validity dossiers.
- Storage: git holds the instrument; frozen inputs and episode tables live in
  object storage (Cloudflare R2), referenced by URL + SHA-256.
- Hugging Face dataset export (one config per series) + Dataset Card.

### Honest scoping
- Documented why a clean area-funding mark is deferred (UKSPF too recent for an
  outcome; the Neighbourhood Renewal Fund running variable is not openly
  reconstructable; the CQC-instrumented fuzzy SHMI first stage is too weak to
  admit). The fuzzy estimator runs on this real data but the mark is not admitted.
