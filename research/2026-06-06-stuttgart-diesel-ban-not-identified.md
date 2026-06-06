# Stuttgart 2019 diesel ban → roadside NO₂: not identified (fleet-renewal + pre-ban-measures collinearity)

*2026-06-06 · candidate `stuttgart-lez-no2-2019` — BUILT then SHELVED (not admitted)*

## Why we tried it

The LEZ→NO₂ bridge ([[bridge-anchor-family-status]]) needs an **interior** anchor: a
ban-type LEZ whose stringency sits between Berlin's mild 2010 green-sticker tightening
(≈null effect) and Madrid Central's near-total core access ban (−11.6 µg/m³). Without a
third anchor the bridge can't exercise leave-one-anchor-out (LOAO) — both endpoints get
skipped, so the headline credibility number is undefined.

Stuttgart's **1 Jan 2019 Euro-4-and-older diesel ban** (city-wide Umweltzone, an access
ban not a charge) looked ideal: a NO₂-targeted diesel ban (intermediate stringency),
pre-COVID, with the famous Am Neckartor hotspot. The raw signal looked strong — the
treated-minus-control roadside excess fell ~11 µg/m³ across the ban.

## Why it fails

Built on the same controlled-ITS machinery as Berlin/Madrid, it is **NOT ADMITTED** —
control parallelism, no-anticipation, and placebo dates all FAIL. The cause is a real,
significant **downward pre-trend** in the treated-minus-control difference, not noise:

- **Background-control spec** (in-zone urban + out-of-zone regional background): pre-trend
  slope **−0.41 µg/m³/month** (se 0.18), placebos fire at +5.0 and +6.8.
- **Roadside-control spec** (traffic stations in Karlsruhe/Mannheim/Freiburg/Heilbronn —
  chosen specifically to net out the nationwide post-Dieselgate diesel fleet-renewal
  trend, which a background control can't): pre-trend slope is **steeper still, −0.48
  µg/m³/month**.

So Stuttgart's roadside NO₂ was already declining **faster than other German cities'
roadside NO₂ throughout 2017–2018, before the ban**. Two collinear drivers explain it,
and neither is separable from the ban under this design:

1. **Post-Dieselgate fleet renewal** cut roadside-diesel NO₂ Europe-wide over 2017–2019;
   old diesels concentrate on busy roads, so roadside fell faster than background — and
   faster at a diesel hotspot. A diesel ban acts on the *same* margin, so the two are
   collinear.
2. **Pre-ban measures at Am Neckartor** — Germany's most-polluted spot carried
   concentrated speed limits, a Feinstaubalarm regime, and intense fleet pressure from
   2018. The formal Jan-2019 ban is one entangled point on a multi-year improvement, not
   a sharp break.

The honest effect under the model is **−3.6 µg/m³ [−8.7, +0.5]** — interval spanning zero,
with a failed validity battery. Declared, not identified.

## The structural lesson for the bridge

This is the **second** LEZ→NO₂ interior candidate to fail identification, after ULEZ
([[ulez-no2-its-build-status]], curved pre-trend). The pattern is structural:

- **Recent (2017–2019) diesel-focused LEZs** are systematically confounded by the
  contemporaneous post-Dieselgate roadside-diesel decline — exactly the margin they act on.
- **Sharp, large, cleanly-identified effects** come from *severe* restrictions (Madrid's
  access ban) → high stringency, not interior.
- **Old (pre-2013) ban-LEZs** are mostly the same green-sticker design as Berlin → clustered
  at *low* stringency, not interior.

So the **interior of the stringency→NO₂ curve is the hardest region to pin from open
data**: the policies that sit there are the ones whose modest effects are most entangled
with secular trends. The two clean anchors we have (Berlin null, Madrid strong) bracket
the curve at its extremes; a clean *middle* anchor has not materialised.

Per the failed-candidate policy, the Stuttgart harvest, frozen inputs, and build wiring are
NOT retained — this note is the sole record. For reproduction: EEA verified (E1a) NO2,
country DE, dataset 2; treated = DEBW118 (Am Neckartor), DEBW116 (Hohenheimer Strasse),
DEBW099 (Arnulf-Klett-Platz); control (roadside spec) = DEBW080 (Karlsruhe), DEBW098
(Mannheim), DEBW122 (Freiburg), DEBW152 (Heilbronn); window 2017-01..2019-12, instant
2019-01-01, on the same controlled-ITS machinery as Berlin/Madrid. Data/licence/dates were
GREEN (EEA E1a, CC BY 4.0) — only identification failed, the recurring theme.
