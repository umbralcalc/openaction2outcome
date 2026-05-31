# openaction2outcome — Spec 1
## Bridge marks: deterministic causal calibration with a discrepancy bridge

### Purpose

Today every mark is a single **identified** real effect (sharp/fuzzy RDD). That confines the collection to policies nature happened to randomise at a clean cutoff. This spec adds a second, clearly-separated *category* of mark — a **bridge mark** — that produces a principled effect estimate *between* two or more real identified marks on the same mechanism, by pinning a causal model to those anchors and spanning the gap with a calibrated discrepancy term.

**Interpolation only.** A bridge mark's query point must lie *inside* the hull of its anchors — bracketed by a real anchor on each side along the policy variable. Extrapolation beyond the anchors is out of scope, by design: a one-sided pin is an unbounded diffusion, not a bridge, and its honest interval cannot be defended. Removing it means every bridge mark is a true bridge — pinned at both ends, bounded throughout — and the project never has to argue "trust the model past where it was certified."

**Deterministic by design.** The honest interval is computed in **closed form** (analytic conditioning) or by **deterministic moment propagation** — never by sampling the discrepancy. This is a load-bearing commitment, not an implementation detail: the instrument's entire value is an exact, re-mintable, honest interval, and stochastic inference silently manufactures over-confident intervals through finite-sample degeneracy (demonstrated below). Sampling-based inference is a *gated, deferred exception* reserved for the few mechanisms that genuinely require it, never the default.

The model is never the source of truth. The real identified marks are the pins; the causal model + a discrepancy term is the span. The honest interval is the posterior: narrow at the pins, bulging between them, always bounded because both ends are fixed. This is Kennedy–O'Hagan Bayesian calibration of computer models, specialised to a bridge geometry, computed deterministically, and expressed in the existing honest-interval framing.

**Non-negotiable principle:** a bridge mark's provenance must make the pin/span boundary unmissable. A consumer must always be able to filter the collection down to identified-only marks and never see a modelled quantity laundered as ground truth. This is the exact discipline that distinguishes openaction2outcome from "evaluate against a simulator," and it cannot be relaxed for convenience.

---

### What a bridge mark *is* (and is not)

**Is:** an estimate of a policy's effect at a point on a mechanism's effect-curve where no clean natural experiment exists, *but which is bracketed by real identified anchors on both sides*, obtained by (a) a deterministic causal model encoding the policy mechanism, (b) calibrated so its discrepancy from truth is pinned to ≥2 real identified marks on the *same* mechanism that bracket the query point, (c) with the honest interval derived in closed form from the posterior discrepancy process.

**Is not:** a model output called "truth"; an *extrapolation* beyond the anchor hull (out of scope entirely); a bridge across anchors that don't lie on one underlying mechanism; an interval produced by a stochastic sampler whose width is a finite-sample artefact.

**Mark categories after this spec:**
- `identified` — the current marks (RDD sharp/fuzzy). Unchanged. The pins. `truth_source = identified`.
- `bridge` — new. Always references ≥2 identified marks that **bracket** the query point on the policy variable. Interpolation only; there is no extrapolation region. `truth_source = simulator-bridged`.

---

### The inference doctrine: determinism first

The calibration must choose *how* to turn the model + anchors into a posterior. There is a ladder of methods, and where a mechanism sits on it decides everything:

1. **Analytic marginalisation (closed form).** When the discrepancy is a Gaussian process entering additively and the anchors are Gaussian observations, the discrepancy integrates out exactly: the posterior over τ(query) is available in closed form. Zero finite-sample randomness, exact, instant, re-mintable. **This is the default and covers a wide class.**
2. **Deterministic moment propagation (unscented / EKF / Laplace).** When the mechanism is *nonlinear but effectively deterministic* with near-Gaussian uncertainty, propagate means and covariances through it with deterministically-placed sigma points — still no Monte Carlo, still re-mintable.
3. **Sampling (particle / EnKF / SMC).** Only when the mechanism is a *genuinely stochastic, intractable-likelihood* simulator, or the posterior is genuinely non-Gaussian. Here finite-sample randomness is mandatory and you only choose which degeneracy you tolerate. **This rung is deferred and gated** (see the tractability gate).

Two independent axes govern the choice, and conflating them is a category error:
- **Axis A — structure needed:** symmetric *association* (a covariance/kernel suffices) vs *directed/causal/interventional* (a generative graph / SCM is required; a covariance cannot answer `do(·)`).
- **Axis B — tractability:** linear-Gaussian (rung 1) / mildly nonlinear (rung 2) / intractable or non-Gaussian (rung 3).

**Randomness is gated by Axis B, not Axis A.** A directed, dynamic, interventional model can be fully closed-form — the Kalman filter is the proof. So "this mechanism is causal" does *not* force sampling; only intractability/non-Gaussianity does. This is why the deterministic regime is far larger than it first appears, and why the project lives there by default.

**Empirical basis (shipped, not asserted).** The sampled-discrepancy joint was built and measured against the closed-form calibrator on identical synthetic problems with known truth (`study --bridge --compare`). Result: the modular closed-form and the *exact* closed-form joint (GP marginal likelihood for θ + analytic δ conditioning) nearly coincide and track nominal coverage (recovery ≈ 1.0 at the 95% level); the SMC-sampled joint degenerates (recovery ≈ 0.25, intervals several-fold too narrow) because the data-free discrepancy latents collapse under resampling and a flexible model over-explains the anchors. The exact joint being analytic means sampling the discrepancy adds *only* Monte-Carlo error and degeneracy. This is the empirical case for the determinism-first doctrine.

---

### The method (what the calibration actually does)

Let a mechanism have a true effect curve τ(x) over a policy variable x (cutoff level, intensity, dose, time). We hold real identified marks at anchor points x₁…xₙ, each a posterior over τ(xᵢ) (the existing honest interval).

1. **Causal model.** A deterministic forward model m(x; θ) of the policy mechanism produces a structured prediction of the effect at any x given parameters θ. It carries the mechanistic content — interactions, feedback, dynamics, and (when expressed as a directed structural graph) interventional structure — that a single RDD cannot. It need not be a black-box stochastic simulator; in the deterministic regime it is a closed-form or moment-propagated map.
2. **Discrepancy bridge.** Model truth as τ(x) = m(x; θ) + δ(x), where δ is a Gaussian process (the discrepancy). δ is *pinned* by requiring τ(xᵢ) to match each anchor's identified posterior, and is conditioned **in closed form**: between bracketing anchors it is a Brownian-bridge-like object — variance → the anchor's own posterior width at the pins (a bridge can never be more certain than its anchors), bulging between them, but always bounded by the kernel prior variance. Query points are admitted only when bracketed; there is no unpinned region.
3. **Inference.** θ is inferred over its well-identified, low-dimensional space (closed-form conditioning, or SMC over θ alone with δ marginalised analytically), and δ(query) is conditioned in closed form — giving a posterior over τ(query). This is the honest interval, and its *shape* is derived from the GP covariance and the model, not asserted.
4. **The covariance kernel is the load-bearing assumption** and ships as provenance. It encodes how fast model-trust decays with distance from an anchor. **The kernel can carry causal structure:** structured and latent-force (ODE-derived) kernels encode a *linearised mechanism*; additive/ARD kernels encode which inputs interact; in the linear-Gaussian limit the covariance *is* the directed graph. Two things a kernel cannot carry, and which therefore mark the boundary of the deterministic regime: **direction/intervention** (a covariance is symmetric — it gives observational conditionals, not `do(·)`) and **global nonlinearity / non-Gaussianity** (a fixed kernel is one linearisation everywhere). The mark records which kernel and which inference rung produced the interval.

---

### The deterministic causal layer (the first major extension)

To maximise the breadth of *defensible* domains, the first major extension augments causal-relationship power **while staying deterministic** — climbing Axis A (more directed/structural content) without climbing Axis B (no sampling). Reusable, closed-form machinery widens coverage across many mechanisms at once; a bespoke stochastic simulator widens it one expensive, harder-to-defend mechanism at a time. The layer is built on stochadex, used for exactly what it is good at:

- **The mechanism `m` as a stochadex deterministic graph.** stochadex's partitions + `ParamForwarding` already form a directed computational graph (nodes + directed edges); `pkg/general` ships deterministic dataflow nodes (`values_function`, `embedded_simulation_run`) and **moment-propagation primitives** (`values_function_vector_mean` / `values_function_vector_covariance`); `analysis` ships a closed-form regression iteration; `continuous` ships an *exact* Ornstein–Uhlenbeck propagator. So a directed, linear-Gaussian (or moment-propagated) structural model — with analytic interventions — is idiomatic stochadex, deterministic, composable, and byte-for-byte re-mintable on the same engine the identified marks already use.
- **Structured / latent-force kernels** for the discrepancy, so a mechanism's known (linearised) structure lives in the covariance, closed-form.
- **A thin Gaussian-conditioning node** (the Kalman/GP update) is the one piece stochadex does not ship — the dense `(K+Σ)⁻¹` solve — added as a deterministic iteration over the existing `internal/bridge` linear algebra.
- **Multi-output / coregionalisation kernels** to bridge correlated outcomes or related jurisdictions jointly.

**The tractability gate (Axis-B detector).** The layer is *determinism-first, not determinism-only*. Past genuine nonlinearity / non-Gaussianity / regime change, a deterministic interval stops being honest and becomes a miscalibrated lie (a tidy Gaussian interval over a multimodal truth). The layer therefore ships an explicit gate that tests whether a mechanism remains in the closed-form / deterministic regime; a mechanism that fails the gate is **not** given a deterministic interval — it is flagged and handed off to the deferred sampling route. The gate verdict ships as provenance.

---

### Repo integration

Reuses the existing offline-mint / static-publish / local-scorer architecture unchanged.

```
internal/bridge        Deterministic calibration: closed-form GP discrepancy conditioning,
                       θ inference (closed-form / SMC-over-θ), LOAO, kernel sensitivity,
                       synthetic recovery study. FOUNDATION BUILT. EXTEND with the
                       deterministic causal layer: stochadex-graph mechanisms, structured/
                       latent-force kernels, deterministic moment propagation, the
                       tractability gate, and (gated, deferred) the sampling route.
internal/series        A bridge series mints a bridge mark from a named family of existing
                       identified marks + a mechanism model.
internal/validity      Bridge-specific checks (below). BUILT; EXTEND with the tractability gate.
internal/dossier       Renders the pin/span picture, the kernel, the bracketing anchors, the
                       anchor-coherence justification, LOAO coverage, and the inference rung. BUILT.
pkg/schema   (public)  The bridge mark category + fields (below). Dependency-light: schema only
                       *describes* a bridge mark; it never runs a model. BUILT.
pkg/score    (public)  Category-aware, never-pooled reporting + an identified/bridge/both filter. BUILT.
marks/                 bridge marks live here too; the category field + naming keep them filterable.
```

### Schema additions (`pkg/schema`) — BUILT

- `category`: `identified` | `bridge` (empty reads as `identified`; existing marks unchanged in value).
- `truth_source`: `identified` | `simulator-bridged` (the hard provenance line).
- For bridge marks (`bridge` block):
  - `anchors`: list of identified-mark IDs + their `policy_point` (≥2, and they must **bracket** `query_point` — one on each side; enforced in the data model at validation time).
  - `policy_variable`: what x is (cutoff level / intensity / dose / time / rank).
  - `query_point`: the x this mark estimates τ at (always inside the anchor hull).
  - `simulator`: model id + version + seed + input hashes (re-mintable byte-for-byte).
  - `discrepancy_kernel`: the GP covariance family + hyperparameters + jitter (the trust-decay assumption, shipped openly).
  - `anchor_coherence`: structured justification that the anchors lie on one mechanism.
- **To add with the deterministic causal layer:** an `inference` record — the rung used (`closed-form` | `deterministic-moment` | `sampled`) and the **tractability-gate verdict** — so a consumer sees exactly which method produced the interval and that determinism was earned, not assumed.

### Validity additions (`internal/validity`)

Bridge marks ship their own battery, analogous to the manipulation check for identified marks:

1. **Anchor coherence (load-bearing).** Justify that all anchors reflect the *same* mechanism (same population, regime, outcome construct). Mandatory; without it the bridge is rejected. BUILT.
2. **Leave-one-anchor-out (LOAO).** Drop each interior anchor, re-fit on the rest, predict the held-out anchor, check its identified posterior falls within the bridge's predicted interval. LOAO coverage is a headline dossier number (the bridge analogue of the identified marks' calibration study). Endpoints cannot be held out without extrapolating and are reported skipped. BUILT.
3. **Kernel sensitivity.** Re-fit under alternative covariance kernels; report how much τ(query) and its interval move. Large movement → kernel-driven, flag prominently. BUILT.
4. **Bracketing enforcement.** The query point must lie strictly between two anchors. Enforced in the data model at mint time — there is no extrapolation fallback. BUILT.
5. **Tractability gate (to add).** Assert the mechanism is in the deterministic regime; otherwise the bridge is flagged and routed to the (deferred) sampling path rather than given a deterministic interval.

### Scoring (`pkg/score`) — BUILT

- Unchanged comparison logic (distribution vs honest interval) works on both categories.
- **Category-aware, never pooled:** identified and bridge marks are reported separately — pooling would let strong identified coverage mask weak bridge coverage.
- A consumer can opt into `identified`-only (maximally defensible), `bridge`-only, or both, with full visibility; filtered marks are reported, never silently dropped.

---

### Honest risk register (carried into the dossier template)

- **Interpolation only — fragility designed out.** Every query point is bracketed, so every honest interval is bounded and pinned at both ends. The unbounded one-sided-diffusion failure mode is removed from scope, not merely flagged.
- **Determinism has a ceiling, and the gate is the guard.** Closed-form / moment-propagated inference is honest only inside the linear-Gaussian / mildly-nonlinear regime. Past it, a deterministic interval is a miscalibrated lie. The tractability gate — not optimism — enforces the boundary; a mechanism that exceeds it is flagged, not papered over.
- **Sampling is quarantined.** The sampled-discrepancy route reintroduces finite-sample degeneracy that manufactures over-confident intervals (measured: recovery ≈ 0.25 vs ≈ 1.0 closed-form at 95%). It is a gated, per-mechanism-justified exception for genuinely intractable/non-Gaussian mechanisms, never the default, and any sampled mark records that fact in provenance.
- **Anchor coherence is an assumption, not a proof.** LOAO tests it empirically but cannot fully guarantee it; the dossier states the coherence argument explicitly so a reader can disagree.
- **The kernel is a choice.** Trust-decay is assumed, not measured; shipping the kernel as provenance is the honesty mechanism. The kernel can encode linearised causal structure but *cannot* encode direction/intervention or global nonlinearity — those are exactly the boundary of the deterministic regime.
- **The binding constraint is anchor families, not modelling power.** No mechanism in the collection yet has ≥2 coherent design-based anchors. Richer deterministic machinery raises the *ceiling* on which families can be bridged; it does not create families. Modelling and harvesting must advance together.
- **Provenance laundering is the cardinal sin.** Any code path that averages a modelled quantity with, or displays it as, an identified one is a bug of the highest severity. The category/truth_source split exists to make this impossible by construction.
- **Centre-of-gravity shift.** Bridge marks reopen, in a controlled way, the "when can a model be trusted off-anchor" debate the identified-only framing sidestepped. The pin/span discipline, interpolation-only scope, LOAO, and the determinism doctrine are the defence; expect scrutiny and lead with the discipline.

---

### Build order

1. **Schema + scorer** (category, truth_source, bracketing in the data model, per-category never-pooled reporting, filtering). **DONE.**
2. **`internal/bridge` foundation on a synthetic mechanism with known τ(x):** closed-form GP discrepancy conditioning, θ inference, LOAO / bracketing / kernel-sensitivity checks, recovery study. **DONE.** The sampled-joint comparison is shipped (`study --bridge --compare`) to demonstrate empirically why the inference is deterministic.
3. **The deterministic causal layer (the first major extension):** the mechanism `m` as a stochadex deterministic graph (directed structural model + analytic interventions), structured/latent-force kernels, deterministic moment propagation, a thin Gaussian-conditioning node, and the tractability gate. Validate on synthetic truth, prove bit-identical re-mints, and prove agreement with the analytic answer in the linear-Gaussian limit.
4. **First real anchor family + first real bridge mark** on a mechanism where the collection has ≥2 bracketing identified anchors (cleanest candidate: the bathing-water band boundaries on one log-compliance-margin running variable — see Spec 2). This is the binding *data* constraint; it raises the floor that the modelling layer raises the ceiling of.
5. **Deferred, gated: the sampling route** (particle / EnKF) for genuinely intractable-likelihood or non-Gaussian mechanisms (the Tier-1 epidemiology case), reached only through the tractability gate, with its degeneracy/inflation handling and its provenance flag.
6. **Dossier rendering + the separate bridge calibration study + README** — the new mark category, the pin/span principle, the determinism doctrine, and per-category calibration reported side by side. **BUILT;** extend to render the inference rung + tractability verdict once step 3 lands.
