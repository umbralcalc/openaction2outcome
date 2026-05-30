# openaction2outcome — Development Plan v3
## Go + stochadex, offline-minted, published as a public repo + static files

*An open causal yardstick mapping actions to verified outcomes.*

**Naming convention (decided):** the project is called **`openaction2outcome`** in all places — repo, Go module, CLI, Hugging Face dataset, brand. No short handle/alias, to remove any "which form do I cite?" ambiguity. (Pending final clash verification against UK Companies House, UK IPO/EUIPO trademarks, the live GitHub org, and the Hugging Face namespace before committing the org + module path.)

**Stack (committed):** application language is **Go**; simulation + inference via **stochadex** and the author's existing Go inference tooling; everything else is off-the-shelf and chosen to minimise maintenance load and cost.

**Governing principle:** this is an **offline dataset that you mint locally and publish as static files.** No hosted service of any kind — no server, no database, no cron, no serverless function, no WASM. You run a Go pipeline by hand; it produces the marks *and* the scores; you push the results to a public GitHub repo and an object store. A consumer who wants to score their own model downloads the same public Go package and runs it locally. Recurring cost: £0. Things that can break while you're not looking: none.

---

## 1. Architecture in one paragraph

A single public **GitHub repo** (`openaction2outcome`) holds the Go module (the "mint"), the vendored input data (or pointers + hashes), the schema, and the published marks + dossiers + scores. You run `openaction2outcome build` on your own machine: it ingests the frozen open data, runs stochadex-based RDD estimation + the validity battery, emits versioned immutable **mark files** (JSON + parquet), and runs the baseline + your stochadex method through the **same scoring package**, writing scores as static files. You commit the outputs and push large binaries to object storage. The Go module is public, so any consumer can `go install` it and score their own model against the marks locally. That's the whole system.

---

## 2. The Go module (one public repo, where stochadex lives)

```
github.com/<you>/openaction2outcome
/cmd/openaction2outcome   # CLI: `build`, `validate`, `score`
/internal/ingest          # per-seam loaders (area funding, floor standards, SHMI)
/internal/rdd             # RDD estimation via stochadex forward model + SBI
/internal/validity        # density (McCrary), covariate-continuity, placebo, donut, first-stage
/internal/mark            # Mark struct, provenance, point-in-time checks
/pkg/schema               # PUBLIC Go types + JSON schema for marks & submissions
/pkg/score                # PUBLIC scoring package (Track A + Track B) — what consumers import
/data/raw                 # vendored frozen inputs (or pointers + SHA-256 if large)
/marks                    # published mark files (JSON) — the dataset
/scores                   # published baseline + reference-method scores (static)
/dossiers                 # rendered validity dossiers (static HTML/markdown)
README.md                 # how to reproduce scores + score your own model
```

Module path: `github.com/<you>/openaction2outcome`. Consumers import e.g. `github.com/<you>/openaction2outcome/pkg/score`. (Longer than a short alias, but consistent everywhere — the deliberate trade-off.)

**Two packages are deliberately public and kept dependency-light** so consumers can use them without pulling the whole minting stack:
- `/pkg/schema` — the Mark + Submission types and JSON schema.
- `/pkg/score` — the scoring logic. It only *compares distributions* (a submission's predicted effect+uncertainty against a mark's honest interval); it does NOT need stochadex or the SBI machinery. Keeping this boundary clean means a consumer importing `score` gets a tiny dependency tree, and the heavy numerical deps stay inside `/internal`. **Verify this boundary in Phase 0.**

**Where stochadex does the work (all inside `/internal`, all offline):** the mark's *honest interval* is a posterior that absorbs identification choices, not a closed-form SE. Use stochadex to define the discontinuity forward model and run SBI over bandwidth / polynomial order / kernel as nuisance parameters → posterior over τ at the cutoff. This is the methodological core and the reason plug-in methods (sampling SE only) fail Track B.
- Sharp seams (area funding, floor standards): local-polynomial discontinuity forward model.
- Fuzzy seam (SHMI): two-stage (first-stage treatment-probability jump, then ratio); SBI propagates both stages into the mark.
- Validity battery: each test returns a structured result shipped *inside* the mark's dossier. Admission = passes validity; never rejected for width.

**Determinism:** seed every stochadex run; record seeds + input hashes + tool versions in each mark's provenance so a mark is re-mintable byte-for-byte. Cheap to enforce now, expensive to retrofit.

---

## 3. Data ingestion (no pipelines, no scheduler)

- **Vendor and freeze the inputs.** Download the specific open files once; store originals (or pointers + SHA-256 if large) with recorded URL, retrieval date, licence. Point-in-time integrity (the leakage requirement) *demands* a frozen vintage anyway, so this is correctness, not laziness.
- **Per-seam loader** = pure functions turning a vendored raw file into a normalised internal table; unit-tested against a committed sample.
- **No ETL service, no scheduler, no DB.** Re-ingest by hand only when you deliberately add a vintage or seam.

---

## 4. Scoring (folded into the local pipeline — no service)

- `openaction2outcome score` runs every published method (plug-in baseline + your stochadex/SBI reference) against the marks using `/pkg/score`, and writes results to `/scores` as static files. You run it locally as part of minting.
- A **consumer** scores their own model by: producing a `submission.json` (their predicted effect + uncertainty per mark, to the published schema), then `go run github.com/<you>/openaction2outcome/cmd/openaction2outcome score --submission submission.json`. Runs on their machine; you host nothing.
- The README documents the submission schema and includes a **committed example submission + its expected scores**, so anyone can verify they're using the tool correctly and reproduce your numbers. This is what makes it a usable *yardstick* rather than a dataset dump.
- **No leaderboard.** If you ever want one, it's a hand-maintained static table in the repo updated from PRs. Yardstick framing makes that acceptable.

---

## 5. Publishing (static, free)

- **Source of truth + dataset + code:** the public GitHub repo `openaction2outcome`. Git gives versioning, immutability, and provenance for free. Tag releases (`v1.0.0`) for citable, frozen snapshots.
- **Large binaries (parquet):** GitHub Releases assets if modest; **Cloudflare R2** if larger — S3-compatible API but **zero egress fees**, the right default for a downloadable dataset. Avoid plain S3 (per-GB download charges) unless already wired and traffic will be trivial.
- **Discoverability mirror:** push the dataset to **Hugging Face Datasets** as `openaction2outcome` — free, the right audience, and it sits directly beside CausalReasoningBenchmark and Open Bandit Dataset (the work you position against / share lineage with). Splits: `floor-standards`, `shmi`, `area-funding`.
- **Dossiers:** render at mint time to static HTML/markdown; view in-repo on GitHub or serve via **GitHub Pages** (free, zero-maintenance). Precompute any discontinuity/posterior plots to static images or JSON — no JS framework.

---

## 6. CI (optional, minimal)

You can run everything by hand. If you want one safety net: a single **GitHub Actions** workflow on tag that runs `openaction2outcome validate` (re-mint and assert outputs match committed hashes → catches accidental drift) + `go test`. No deploys, no scheduled jobs, no self-hosted runners. Nothing runs unless you push a tag. Optional; manual is fine for v1.

---

## 7. Cost & maintenance summary

| Component | Tech | Cost | Maintenance |
|---|---|---|---|
| Mint + scoring (offline) | Go + stochadex, run by hand | £0 (your machine) | Run on demand |
| Code + dataset + scores | Public GitHub repo | £0 | None |
| Large binaries | GitHub Releases / Cloudflare R2 | £0 (R2 zero-egress) | None |
| Discoverability mirror | Hugging Face Datasets | £0 | None |
| Dossiers | Static HTML (in-repo or Pages) | £0 | None |
| Consumer scoring | They `go install` the public module | £0 to you | None |
| CI (optional) | GitHub Actions on tag | £0 | One workflow file |

**Recurring cost: £0. Hosted services: none. Databases: none. Scheduled jobs: none. WASM/Workers: none.** The system is inert between the times you choose to run it.

---

## 8. Build phases (each ends in something shippable)

**Phase 0 — Schema + one hand-made mark.** Define the `Mark` + `Submission` types and JSON schema in `/pkg/schema` (extend the CausalReasoningBenchmark identification schema with action / alternative-action / decision-value fields). Hand-build one area-funding mark end-to-end with a simple local-linear fit (no stochadex yet) to nail schema, provenance, and point-in-time fields. Verify the `score`-vs-`internal` package boundary holds (scoring imports nothing heavy). *Deliverable: one valid mark + schema + the public module skeleton.*

**Phase 1 — stochadex RDD + validity battery on the anchor seam.** Build `/internal/rdd` (SBI → honest interval) and `/internal/validity`. Run on **area funding (UKSPF/IMD)** first — non-manipulable running variable makes it the gentlest test of the estimation code. *Deliverable: a handful of fully-dossiered area-funding marks with posterior intervals.*

**Phase 2 — scoring + the headline finding.** Implement `/pkg/score` (Track A + Track B), wire `openaction2outcome score`, run the plug-in baseline + your stochadex method, and *show the calibration gap* between them in `/scores`. *Deliverable: reproducible scores + a committed example submission; the headline result is demonstrable on one clean seam.*

**Phase 3 — second + third seams.** Add floor standards (sharp, mildly manipulable) and SHMI (fuzzy, two-stage) — each exercises new code (manipulation-sensitivity; first-stage). *Deliverable: three-domain corpus, v1 complete.*

**Phase 4 — publish + write-up.** Tag `v1.0.0`, push to R2 + Hugging Face, render dossiers, write the "Engineering Smart Actions in Practice" post (harvest method, validity protocol, open-vs-SRS scoping, the calibration finding). *Deliverable: public v1 + post.*

Phases 0–2 are the spine: at the end of Phase 2 you have a complete, novel, working instrument on one seam, published as a public repo. Phases 3–4 add breadth and reach.

---

## 9. Deferred to build-time (not blockers)
- Exact scoring rule / commensurability treatment (brief §5) — settle in Phase 2 once real marks + baseline behaviour are visible.
- Per-seam mark yield — discovered by running Phases 1/3.
- Final name clash verification (Companies House, trademark, GitHub org, HF namespace) — before committing the org + module path.

---

## 10. Anti-scope-creep guardrails
- No live ingestion, ever, in v1 — vendor and freeze.
- No database, no auth, no accounts, no leaderboard service.
- No hosted scoring (no WASM, no Worker) — scoring runs where the data is.
- Keep `/pkg/score` and `/pkg/schema` dependency-light and public; heavy deps stay in `/internal`.
- Re-minting is manual and deterministic; nothing auto-updates.
- One name everywhere: `openaction2outcome`. No aliases.