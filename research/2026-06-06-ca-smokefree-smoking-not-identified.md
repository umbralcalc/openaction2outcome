# Canadian provincial smoke-free / tobacco-tax → smoking prevalence: not cleanly identifiable from open CCHS data

*2026-06-06 · PLANS round-2 series 8 (`ca-smokefree-smoking`) — de-risked, NOT minted*

Series 8 was scoped as the "lowest marginal ingest cost" round-2 mark: reuse the
StatCan CCHS province×year smoking-prevalence panel already built for the menthol mark
([[ca-menthol-did-mark]]) and add a second Canadian anchor on the broader
"tobacco-control intensity → smoking" mechanism, via either provincial **comprehensive
smoke-free public-place laws** or **sharp provincial tobacco-tax step changes** (DiD).
Data, licence and dates were all GREEN (same StatCan Open Licence source, table
13-10-0451 reaches back to 2003). It fails on **identification**, in both sub-routes.

## Sub-route A — smoke-free public-place laws: the adoption wave predates the annual CCHS

Canada's provincial 100%-smoke-free public-place laws rolled out in a tight wave
**2003–2008**: NB & MB (Oct 2004), SK (Jan 2005), NL (Jul 2005), ON & QC (31 May 2006),
NS (Dec 2006), AB & BC (Jan/Mar 2008). By 2008 **every province is treated** — there is
no never-treated control province (the territories aren't in the 10-province table).

The CCHS smoking series exists only at cycles **2003, 2005, then annually 2007+** — the
biennial early cycles **miss the 2004 and 2006 transition years entirely**. The only
clean 2×2 the data permits is:

- **Treated** = ON, QC (adopt 31 May 2006); **Control** = AB, BC (not treated until 2008).
- **Pre** = {2003, 2005}; **Post** = {2007} (2006 is the ramp; 2008 contaminates the controls).

Eyeballed from the frozen panel (current-smoker %, both sexes, 12+):

| prov | 2003 | 2005 | 2007 | pre-mean | Δ(post−pre) |
|---|---|---|---|---|---|
| ON | 22.3 | 20.9 | 20.8 | 21.6 | −0.8 |
| QC | 26.0 | 24.4 | 25.1 | 25.2 | −0.1 |
| AB | 23.0 | 22.8 | 22.0 | 22.9 | −0.9 |
| BC | 18.8 | 17.8 | 17.8 | 18.3 | −0.5 |

Treated Δ ≈ −0.45, control Δ ≈ −0.70 → **DiD ≈ +0.25 pp** — a null. Worse, the design is
**not identifiable**: with two pre points (2003, 2005) and one post point (2007) there is
no room for a pre-trend slope test or placebo-date battery, so the parallel-trends
assumption can't be defended — the menthol mark was admitted precisely *because* its
2007–2014 pre-period let those checks run. Adding NS (Dec 2006) to the treated cohort
swings the estimate by **+1.25 pp** on a single year's noise (NS 22.7→24.4, 2005→2007),
confirming the estimate is dominated by sampling noise, not signal. A mark here would be
neither a clean effect nor a defensible honest null — it fails the validity bar, not just
the effect-size bar.

This is the same lesson the PLAN's own reproducibility note flagged for the UK 2007
smoke-free design: **a policy whose natural experiment predates the open outcome series
is not reproducible from open data.** Canada's smoke-free wave (2003–2008) sits in the
CCHS's sparse biennial era; the design needs ~2001–2006 annual data that doesn't exist.

## Sub-route B — tobacco-tax step changes: swamped by annual prevalence noise

The tax route uses the data-rich annual window (2007–2019), but the prevalence response to
a single provincial tax step is sub-pp and lost in the ±2–3 pp year-to-year noise of the
provincial series. Alberta's large **+$5/carton** hike (Oct 2015, NDP budget) is the
sharpest candidate: AB current-smoking goes 19.0 (2014) → 18.4 (2015) → 18.3 (2016) →
16.6 (2017) — a continuation of the steady national decline that *control* provinces show
over the same years (ON 17.5→16.0→15.4), with **no separable step at the hike**. Most
provinces also raised tobacco taxes somewhere in 2015–2017, so "not-yet-treated control"
is muddy. And there is **no cleanly openly-licensed consolidated provincial tobacco-tax
rate table** (the maintained historical series are compiled by advocacy NGOs with unclear
redistribution terms) — so even the treatment variable isn't GREEN, unlike the outcome.

## Verdict

`ca-smokefree-smoking` is **not minted**. The Canadian "tobacco-control intensity →
smoking" family stays at **one anchor** (the menthol ban). Smoke-free-law identification is
blocked by the CCHS data window; the tax route is blocked by effect/noise separability and
an un-open treatment source. Revisit only if (a) a restricted-access annual pre-2007 CCHS
smoking series becomes usable, or (b) a single province runs a tax step large and isolated
enough to clear the annual noise, with an openly-licensed rate table. Data/licence/dates
were GREEN — only identification failed, the recurring theme of these seams
([[bridge-anchor-family-status]], [[us-flavour-ban-null]], [[naloxone-honest-width-showcase]]).
