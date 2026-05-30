# openaction2outcome — Spec 1
## Bridge marks: Bayesian calibration with a discrepancy bridge

### Purpose

Today every mark is a single **identified** real effect (sharp/fuzzy RDD). That confines the collection to policies nature happened to randomise at a clean cutoff. This spec adds a second, clearly-separated *category* of mark — a **bridge mark** — that uses a stochadex simulator pinned to two or more real identified marks on the same mechanism to produce principled effect estimates *between* those anchors.

**Interpolation only.** A bridge mark's query point must lie *inside* the hull of its anchors — bracketed by a real anchor on each side along the policy variable. Extrapolation beyond the anchors is out of scope, by design: a one-sided pin is an unbounded diffusion, not a bridge, and its honest interval cannot be defended. Removing it means every bridge mark is a true bridge — pinned at both ends, bounded throughout — and the project never has to argue "trust the simulator past where it was certified."

The simulator is never the source of truth. The real identified marks are the pins; the simulator + a Gaussian-process discrepancy term is the span. The honest interval is the posterior: narrow at the pins, bulging between them, always bounded because both ends are fixed. This is Kennedy–O'Hagan Bayesian calibration of computer models, specialised to a bridge geometry and expressed in the existing honest-interval framing.

**Non-negotiable principle:** a bridge mark's provenance must make the pin/span boundary unmissable. A consumer must always be able to filter the collection down to identified-only marks and never see a simulated quantity laundered as ground truth. This is the exact discipline that distinguishes openaction2outcome from "evaluate against a simulator," and it cannot be relaxed for convenience.

---

### What a bridge mark *is* (and is not)

**Is:** an estimate of a policy's effect at a point on a mechanism's effect-curve where no clean natural experiment exists, *but which is bracketed by real identified anchors on both sides*, obtained by (a) a stochadex simulator encoding the policy mechanism, (b) calibrated so its discrepancy from truth is pinned to ≥2 real identified marks on the *same* mechanism that bracket the query point, (c) with the honest interval derived from the posterior discrepancy process.

**Is not:** a simulator output called "truth"; an *extrapolation* beyond the anchor hull (out of scope entirely); a bridge across anchors that don't lie on one underlying mechanism.

**Mark categories after this spec:**
- `identified` — the current marks (RDD sharp/fuzzy). Unchanged. The pins.
- `bridge` — new. Always references ≥2 identified marks that **bracket** the query point on the policy variable. Interpolation only; there is no extrapolation region.

---

### The method (what stochadex + SBI actually do)

Let a mechanism have a true effect curve τ(x) over a policy variable x (cutoff level, intensity, dose, time). We hold real identified marks at anchor points x₁…xₙ, each a posterior over τ(xᵢ) (the existing honest interval).

1. **Simulator.** A stochadex forward model m(x; θ) of the policy mechanism produces a structured prediction of the effect at any x given parameters θ. This carries the mechanistic content — interactions, feedback, spillovers, dynamics — that a single RDD cannot.
2. **Discrepancy bridge.** Model truth as τ(x) = m(x; θ) + δ(x), where δ is a Gaussian process (the discrepancy). δ is *pinned* by requiring τ(xᵢ) to match each anchor's identified posterior. Between bracketing anchors δ is a Brownian-bridge-like object: variance → 0 at the pins, bulging between them, but always bounded because both ends are fixed. Query points are admitted only when they fall between anchors; there is no unpinned region.
3. **Inference.** SBI over (θ, δ) given the anchor posteriors → a posterior over τ(x) at any queried x. This is the honest interval for the bridge mark, and its *shape* is now derived from the GP covariance, not asserted.
4. **The covariance kernel is the load-bearing assumption** and ships as provenance. It encodes how fast simulator-trust decays with distance from an anchor. Different kernels → different widening laws; the mark records which was used so consumers see exactly what trust-decay assumption the estimate rests on.

---

### Repo integration

Reuses the existing offline-mint / static-publish / local-scorer architecture unchanged. New and changed pieces:

```
internal/bridge        NEW. simulator harness + GP discrepancy + SBI calibration.
                       depends on stochadex; stays in internal (heavy deps).
internal/series        EXTEND. a bridge series mints a bridge mark from a named
                       family of existing identified marks + a mechanism simulator.
internal/validity      EXTEND. add bridge-specific checks (below).
internal/dossier      EXTEND. render the pin/span picture, the kernel, the
                       bracketing anchors, the anchor-coherence justification.
pkg/schema   (public)  EXTEND. add the bridge mark category + fields (below).
                       MUST stay dependency-light: schema only *describes* a bridge
                       mark; it never runs the simulator.
pkg/score    (public)  EXTEND. scoring already compares a model's predicted
                       distribution to a mark's honest interval — works unchanged
                       on bridge marks. ADD: a filter so a consumer can score
                       against identified-only, bridge-only, or both, and so the
                       headline calibration study can be reported separately per
                       category (never pooled silently).
marks/                 bridge marks live here too, but the category field and a
                       naming convention (e.g. bridge/ subdir) keep them filterable.
```

### Schema additions (`pkg/schema`)

- `category`: `identified` | `bridge` (new required field; existing marks default to `identified`).
- `truth_source`: `identified` | `simulator-bridged` (the hard provenance line; bridge marks are always the latter).
- For bridge marks:
  - `anchors`: list of identified-mark IDs this bridge is pinned to (≥2, and they must **bracket** `query_point` on the policy variable — one anchor on each side).
  - `policy_variable`: what x is (cutoff level / intensity / dose / time).
  - `query_point`: the x this mark estimates τ at (always inside the anchor hull).
  - `simulator`: stochadex model id + version + seed + input hashes (re-mintable byte-for-byte, same determinism rule as identified marks).
  - `discrepancy_kernel`: the GP covariance family + hyperparameters (the trust-decay assumption, shipped openly).
  - `anchor_coherence`: structured justification that the anchors lie on one mechanism (see validity).

### Validity additions (`internal/validity`)

Bridge marks need their own battery, analogous to the manipulation check for identified marks:

1. **Anchor coherence (the bridge-specific load-bearing check).** Justify that all anchors reflect the *same* underlying mechanism (same population definition, same policy regime, same outcome construct). A bridge across anchors from different causal regimes is a category error — a smooth path between points not on one curve. This justification is mandatory and shipped in the dossier; without it the bridge is rejected.
2. **Leave-one-anchor-out (LOAO) validation.** Drop each anchor in turn, re-fit the bridge on the rest, predict the held-out anchor, check its identified posterior falls within the bridge's predicted interval. This is the bridge's empirical credibility test — it directly measures whether the simulator+discrepancy actually interpolates real truth. LOAO coverage ships as a headline dossier number (the bridge analogue of the identified marks' calibration study).
3. **Kernel sensitivity.** Re-fit under alternative covariance kernels; report how much τ(query_point) and its interval move. Large movement → the estimate is kernel-driven, flag prominently.
4. **Bracketing enforcement.** The query point must lie strictly between two anchors on the policy variable. A bridge mark whose query point is not bracketed is rejected at mint time — there is no extrapolation path to fall back to. This is enforced in the data model, not left to dossier discretion.

### Scoring (`pkg/score`)

- Unchanged comparison logic (distribution vs honest interval).
- New: category-aware reporting. The calibration study reports identified and bridge marks **separately, never pooled** — pooling would let strong identified coverage mask weak bridge coverage. A consumer scoring a model gets per-category breakdowns.
- New: a consumer can opt into `identified`-only for a maximally-defensible test, or include `bridge` marks for broader-reach evaluation, with full visibility.

---

### Honest risk register (carried into the dossier template)

- **Interpolation only — fragility designed out.** Because every query point is bracketed by real anchors, every honest interval is bounded and pinned at both ends. The unbounded one-sided-diffusion failure mode is removed from scope entirely, not merely flagged. The defensible mode is the *only* mode.
- **Anchor coherence is an assumption, not a proof.** LOAO tests it empirically but cannot fully guarantee it; the dossier states the coherence argument explicitly so a reader can disagree.
- **The kernel is a choice.** Trust-decay is assumed, not measured; shipping the kernel as provenance is the honesty mechanism, not a solution.
- **Centre-of-gravity shift.** Adding bridge marks reopens, in a controlled way, the "when can a simulator be trusted off-anchor" debate the identified-only framing sidestepped. The pin/span discipline and LOAO are the defence; expect scrutiny and lead with the discipline.
- **Provenance laundering is the cardinal sin.** Any code path that lets a simulated quantity be averaged with, or displayed as, an identified one is a bug of the highest severity. The category/truth_source split exists to make this impossible by construction.

---

### Build order

1. Schema + scorer changes first (category, truth_source, filtering, per-category reporting) — small, and they let everything downstream stay filterable.
2. `internal/bridge` on a *synthetic* mechanism with known τ(x): pin to fake bracketing anchors, confirm the bridge recovers the known curve between them and that LOAO/bracketing/kernel checks behave. This validates the machinery before any real mechanism.
3. First real bridge mark on a mechanism where the existing collection already has ≥2 identified anchors (the cleanest candidate is the one with the most repeat-structure — see Spec 2 for which domains naturally yield anchor families).
4. Dossier rendering + the separate bridge calibration study.
5. README: new mark category, the pin/span principle stated prominently, per-category calibration reported side by side.