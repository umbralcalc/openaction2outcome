---
license: other
license_name: ogl-3.0-and-mit
license_link: https://github.com/umbralcalc/openaction2outcome/blob/main/LICENSE
pretty_name: "OpenAction2Outcome: an open causal yardstick"
tags:
- causal-inference
- regression-discontinuity
- counterfactual
- evaluation
- uncertainty-quantification
size_categories:
- n<1K
configs:
- config_name: floor-standards
  data_files:
  - split: test
    path: floor_standards.jsonl
- config_name: shmi
  data_files:
  - split: test
    path: shmi.jsonl
---

# OpenAction2Outcome — real reference points for testing counterfactual models

If your model answers *"what would happen if we did X?"* — a world model, a
model-based policy, a digital twin, an LLM reasoning about consequences — you need
to know when it is right. Usually you can't: real-world counterfactual ground
truth is rarely available, so people fall back on simulators.

This dataset provides that ground truth. Each **mark** is a real public-sector
decision where an institution crossed a published threshold (a school's
performance score, a hospital's mortality rating), which triggered an action, and
where the **true effect** of that action is recovered by regression discontinuity
— because units that *just* crossed the line are comparable to those that *just*
didn't. Every mark ships with an **honest interval**: a central estimate plus a
range that is truthful about what is genuinely uncertain (it separates ordinary
sampling error from the identification uncertainty of modelling choices).

You can check two things against a mark: does your model get the **effect** right,
and is its **confidence** honest?

This is a small, deliberately bulletproof *reference instrument* (a yardstick
needs accurate marks, not many), not a leaderboard.

## The marks

| id | domain | unit | design | effect (95% interval) |
|---|---|---|---|---|
| `floor-standards-p8-2016` | Education | school | sharp RDD on the 2016 Progress 8 floor (−0.5) | 0.028 [−0.054, 0.256] |
| `shmi-higher-than-expected-banding` | Health | NHS trust | sharp ITT on the SHMI "higher than expected" banding | −0.013 [−0.066, 0.018] |

Each series is a separate, individually-loadable subset (config).

## Load it

```python
from datasets import load_dataset, get_dataset_config_names

get_dataset_config_names("umbralcalc/openaction2outcome")  # ['floor-standards', 'shmi']

ds = load_dataset("umbralcalc/openaction2outcome", "floor-standards")
mark = ds["test"][0]
mark["effect_central"], mark["effect_lower"], mark["effect_upper"]
```

Each row carries the decision setup (`running_variable`, `cutoff`, `action`,
`alternative`, `outcome`), the effect distribution (`effect_central`,
`effect_lower/upper`, `effect_std_dev`, `effect_quantiles`, `effect_samples`, and
the `effect_sampling_sd` vs `effect_identification_sd` split), the validity
verdict (`admitted`), and a link to the full per-unit **episode table**
(`episode_table_url` + `episode_table_sha256`) for those who want the raw rows.
Full field reference: [docs/schema.md](https://github.com/umbralcalc/openaction2outcome/blob/main/docs/schema.md).

## Score your model

The dataset ships with a small, dependency-light Go scorer
([`pkg/score`](https://github.com/umbralcalc/openaction2outcome/tree/main/pkg/score)).
Produce a `submission.json` with your predicted effect *and your own uncertainty*
per mark, then:

```sh
go run github.com/umbralcalc/openaction2outcome/cmd/openaction2outcome score \
  --submission submission.json
```

You get two independent scores per mark:

- **Decision** — did you get the direction of the effect right, and what would a
  wrong call cost? (No penalty where the reference itself is unsure of the sign.)
- **Calibration** — does your stated uncertainty match the truth? Interval
  overlap, a distribution distance, a calibration curve, and a *confidently
  wrong* flag (narrow-and-wrong while the reference is narrow-and-known).

## The finding it is built to show

A method that reports only its **sampling** error looks confident but is wrong too
often — it omits the **identification** uncertainty from modelling choices. The
companion calibration study (against *known* truth, 100 synthetic problems) shows
it clearly: at a nominal 95% interval, a plug-in method covers the truth only
**80%** of the time, while the model-averaged method covers **92%**. The marks
carry that identification uncertainty so the gap is measurable.

## Provenance & licence

Built from UK open data (© Crown copyright) under the
[Open Government Licence v3.0](https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/):
DfE Key Stage 4 performance tables and NHS England SHMI. Each mark records its
sources, licences, and point-in-time timestamps (`context_asof ≤ decision <
outcome`). The schema and scorer are MIT-licensed. Marks are minted
**deterministically** — same inputs, same bytes out.

- Code, full marks, dossiers, and scorer: <https://github.com/umbralcalc/openaction2outcome>
- Per-mark validity dossiers: [dossiers/](https://github.com/umbralcalc/openaction2outcome/tree/main/dossiers)

## Citation

```bibtex
@misc{openaction2outcome,
  title  = {OpenAction2Outcome: an open causal yardstick for testing counterfactual models},
  author = {Hardwick, Robert},
  year   = {2026},
  url    = {https://github.com/umbralcalc/openaction2outcome}
}
```
