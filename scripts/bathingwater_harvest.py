#!/usr/bin/env python3
"""One-time bathing-water harvest for the (sharp RDD) bathing-water seam.

The Environment Agency publishes designated bathing-water data ONLY through the
Defra linked-data API (environment.data.gov.uk) under the revised Bathing Water
Directive (rBWD) regime — there is no single bulk CSV upstream. This script
harvests the API once and assembles two frozen, deterministic CSVs that become
the canonical inputs (hash-pinned, committed as SOURCE.json pointers, mirrored to
object storage). The Go build reads only the frozen CSVs; re-running this needs
network and may differ as the EA revises historical vintages.

Outputs (under OUT_DIR):

  bathing_water_compliance.csv   one row per (sampling point, season year):
      site_id, eubwid, name, water_type, region, district,
      sewerage_undertaker, year_designated, impacted_by_heavy_rain,
      season_year, classification_code, classification_name,
      first_sample_date, final_sample_date,
      ec_p90, ec_p95, ec_sample_count, ec_mean_log10, ec_sd_log10,
      ie_p90, ie_p95, ie_sample_count, ie_mean_log10, ie_sd_log10

  bathing_water_samples.csv      per-sample microbiology for the 4-year windows
      of every NEAR-CUTOFF site-year (classified Poor or Sufficient — the two
      bands adjacent to the action-triggering Poor/Sufficient boundary), to
      support the abnormal-sample-exclusion sensitivity check:
      site_id, sample_year, sample_date_time, ec_count, ec_qualifier,
      ie_count, ie_qualifier, discountable

The Poor/Sufficient boundary (the action-triggering cutoff) is set by the 90th
percentile: coastal IE<=185 & EC<=500 cfu/100ml; inland IE<=330 & EC<=900.
A water is Poor iff it fails Sufficient. The running variable derived downstream
is the log compliance margin = max(log10(EC_p90/EC_thr), log10(IE_p90/IE_thr)).

Every API response is cached under OUT_DIR/.cache so the harvest is resumable:
re-running skips anything already fetched. Network is driven through curl (the
sandbox blocks Python's own sockets), mirroring scripts/cqc_extract.py.
"""
import csv
import hashlib
import json
import os
import subprocess
import sys
import time

YEARS = list(range(2015, 2025))  # rBWD regime; 2020 is a COVID gap (skipped if empty)
# Per-sample microbiology is harvested only for the rolling windows of the
# decision cohorts the marks actually ship on (the sensitivity check operates on
# the decision-year running variable). This keeps the per-sample pull bounded;
# re-run with more years here to support future cohort marks.
DECISION_YEARS = {2015}
BASE = "https://environment.data.gov.uk/data/bathing-water-quality"
ENT = "https://environment.data.gov.uk/id/bathing-water/"
# Each frozen CSV lives in its own source_id cache dir (matching the SOURCE.json
# r2_object_key layout raw/<source_id>/<local_path>). The raw per-call API
# responses are cached in a shared scratch dir so the harvest is resumable.
COMP_DIR = "data/cache/bathing-water-rbwd-2015-2024"
SAMP_DIR = "data/cache/bathing-water-samples-2015-2024"
CACHE = os.path.join(COMP_DIR, ".cache")
SLEEP = 0.05  # politeness between uncached calls

# rBWD compliance-classification codes.
CC = {"1": "Excellent", "2": "Good", "3": "Sufficient", "4": "Poor",
      "5": "InsufficientlySampled", "6": "New", "11": "Closed"}
NEAR_CUTOFF = {"Poor", "Sufficient"}


def curl(url):
    r = subprocess.run(["curl", "-sS", "-L", "--max-time", "120", url],
                       capture_output=True, text=True)
    return r.stdout


def fetch_json(url):
    """GET url (?_format=json), with an on-disk cache keyed by URL hash."""
    key = hashlib.sha256(url.encode()).hexdigest()[:24]
    path = os.path.join(CACHE, key + ".json")
    if os.path.exists(path):
        with open(path) as f:
            return json.load(f)
    sep = "&" if "?" in url else "?"
    body = curl(url + sep + "_format=json")
    try:
        d = json.loads(body)
    except Exception:
        return None  # transient / HTML error page — caller retries or skips
    with open(path, "w") as f:
        json.dump(d, f)
    time.sleep(SLEEP)
    return d


def fetch_json_retry(url, tries=4):
    for _ in range(tries):
        d = fetch_json(url)
        if d is not None:
            return d
        time.sleep(1.0)
    return None


def lit(v):
    """Unwrap a linked-data literal (which may be a dict, list, or scalar)."""
    if isinstance(v, dict):
        return v.get("_value", v.get("_about", ""))
    if isinstance(v, list):
        return lit(v[0]) if v else ""
    return v if v is not None else ""


def point_of(about):
    return about.split("/point/")[1].split("/year/")[0]


def water_type(types):
    """Map the rBWD bathing-water subtype to the threshold class. Coastal AND
    transitional (estuarine) waters share the coastal Sufficient thresholds;
    river and lake waters use the looser inland thresholds."""
    t = " ".join(types) if isinstance(types, list) else str(types)
    if "River" in t or "Lake" in t:
        return "inland"
    if "Coastal" in t or "Transitional" in t:
        return "coastal"
    return "unknown"


def enumerate_site_years():
    """All (point, year, classification) via the per-year compliance slices."""
    rows = []
    for y in YEARS:
        d = fetch_json_retry(f"{BASE}/compliance-rBWD/slice/year/{y}")
        if not d:
            print(f"  {y}: no slice (skipped)")
            continue
        obs = d["result"]["primaryTopic"].get("observation", [])
        for o in obs:
            code = o["complianceClassification"]["_about"].rsplit("/", 1)[1]
            rows.append({
                "site_id": point_of(o["_about"]),
                "eubwid": lit(o["bwq_bathingWater"].get("_about", "")).rsplit("/", 1)[-1],
                "name": lit(o["bwq_bathingWater"].get("name", "")),
                "season_year": y,
                "classification_code": code,
                "classification_name": CC.get(code, code),
            })
        print(f"  {y}: {len(obs)} site-years")
    return rows


def water_meta(site_ids, eubwid_by_site):
    """Per-water static metadata, joined on the STABLE sampling-point notation.

    Sourced from the bathing-water list endpoint (doc/bathing-water), which
    enumerates every currently designated water with its subtype (Coastal /
    Transitional / River / Lake), region, district, sewerage undertaker, year
    designated and heavy-rain flag. This is robust where the per-year eubwid has
    been renumbered: the sampling-point id is invariant. Waters de-designated
    before the present (a handful of historical coastal sites) are absent from
    the list and fall back to the coastal threshold default downstream."""
    meta = {}
    page = 0
    while page < 20:
        d = fetch_json_retry(f"https://environment.data.gov.uk/doc/bathing-water.json?_pageSize=200&_page={page}")
        items = (d or {}).get("result", {}).get("items", [])
        if not items:
            break
        for o in items:
            sp = o.get("samplingPoint") or {}
            sid = (sp.get("_about", "") if isinstance(sp, dict) else "").rsplit("/", 1)[-1]
            if not sid:
                continue
            meta[sid] = {
                "water_type": water_type(o.get("type", [])),
                "region": lit(o.get("regionalOrganization", {}).get("name", "")) if isinstance(o.get("regionalOrganization"), dict) else "",
                "district": lit(o.get("district", "")),
                "sewerage_undertaker": lit(o.get("appointedSewerageUndertaker", {}).get("_about", "")).rsplit("/", 1)[-1] if isinstance(o.get("appointedSewerageUndertaker"), dict) else "",
                "year_designated": lit(o.get("yearDesignated", "")).rsplit("/", 1)[-1],
                "impacted_by_heavy_rain": lit(o.get("waterQualityImpactedByHeavyRain", "")),
            }
        print(f"    water-list page {page}: {len(meta)} waters mapped so far")
        page += 1
    # Sites present in the compliance series but absent from the current list
    # (de-designated) default to coastal thresholds.
    for sid in site_ids:
        meta.setdefault(sid, {"water_type": "coastal"})
    return meta


def stats(site_id, year, measure):
    """SummaryStatistics (percentiles, log mean/sd, sample count) for one measure."""
    d = fetch_json_retry(f"{BASE}/compliance-rBWD/point/{site_id}/year/{year}/{measure}")
    if not d:
        return {}
    pt = d["result"]["primaryTopic"]
    return {
        "p90": lit(pt.get("percentile90", "")),
        "p95": lit(pt.get("percentile95", "")),
        "n": lit(pt.get("sampleCount", "")),
        "mean_log10": lit(pt.get("meanLog10Value", "")),
        "sd_log10": lit(pt.get("stdDeviationLog10", "")),
    }


def compliance_detail(site_id, year):
    """First/final sample dates from the compliance assessment item."""
    d = fetch_json_retry(f"{BASE}/compliance-rBWD/point/{site_id}/year/{year}")
    if not d:
        return {}
    pt = d["result"]["primaryTopic"]
    return {
        "first_sample_date": lit(pt.get("firstSampleDate", "")),
        "final_sample_date": lit(pt.get("finalSampleDate", "")),
    }


def samples_for(site_id, year):
    """All per-sample EC/IE counts + discountable flag for one point-year."""
    d = fetch_json_retry(f"{BASE}/in-season/slice/point/{site_id}/year/{year}")
    if not d:
        return []
    members = d["result"]["primaryTopic"].get("observation", [])
    out = []
    for m in members:
        si = fetch_json_retry(m["_about"])
        if not si:
            continue
        s = si["result"]["primaryTopic"]
        out.append({
            "site_id": site_id,
            "sample_year": year,
            "sample_date_time": lit(s.get("sampleDateTime", {}).get("inXSDDateTime", "")) if isinstance(s.get("sampleDateTime"), dict) else "",
            "ec_count": s.get("escherichiaColiCount", ""),
            "ec_qualifier": lit(s.get("escherichiaColiQualifier", {}).get("countQualifierNotation", "")) if isinstance(s.get("escherichiaColiQualifier"), dict) else "",
            "ie_count": s.get("intestinalEnterococciCount", ""),
            "ie_qualifier": lit(s.get("intestinalEnterococciQualifier", {}).get("countQualifierNotation", "")) if isinstance(s.get("intestinalEnterococciQualifier"), dict) else "",
            "discountable": lit(s.get("discountable", "")),
        })
    return out


COMPLIANCE_COLS = [
    "site_id", "eubwid", "name", "water_type", "region", "district",
    "sewerage_undertaker", "year_designated", "impacted_by_heavy_rain",
    "season_year", "classification_code", "classification_name",
    "first_sample_date", "final_sample_date",
    "ec_p90", "ec_p95", "ec_sample_count", "ec_mean_log10", "ec_sd_log10",
    "ie_p90", "ie_p95", "ie_sample_count", "ie_mean_log10", "ie_sd_log10",
]
SAMPLE_COLS = [
    "site_id", "sample_year", "sample_date_time",
    "ec_count", "ec_qualifier", "ie_count", "ie_qualifier", "discountable",
]


def main():
    os.makedirs(CACHE, exist_ok=True)
    os.makedirs(SAMP_DIR, exist_ok=True)

    print("[1/4] enumerating site-years 2015-2024 ...")
    site_years = enumerate_site_years()
    site_ids = {r["site_id"] for r in site_years}
    eubwid_by_site = {r["site_id"]: r["eubwid"] for r in site_years if r["eubwid"]}
    print(f"  {len(site_years)} site-years across {len(site_ids)} sampling points")

    print("[2/4] fetching per-water metadata ...")
    meta = water_meta(site_ids, eubwid_by_site)

    print("[3/4] fetching EC/IE percentile statistics per site-year ...")
    comp_rows = []
    for i, r in enumerate(site_years):
        sid, y = r["site_id"], r["season_year"]
        if r["classification_name"] in ("Closed", "New"):
            # no microbiology assessment for these
            pass
        ec = stats(sid, y, "escherichiaColiStats")
        ie = stats(sid, y, "intestinalEnterococciStats")
        det = compliance_detail(sid, y)
        m = meta.get(sid, {})
        comp_rows.append({
            **r, **m, **det,
            "ec_p90": ec.get("p90", ""), "ec_p95": ec.get("p95", ""),
            "ec_sample_count": ec.get("n", ""), "ec_mean_log10": ec.get("mean_log10", ""),
            "ec_sd_log10": ec.get("sd_log10", ""),
            "ie_p90": ie.get("p90", ""), "ie_p95": ie.get("p95", ""),
            "ie_sample_count": ie.get("n", ""), "ie_mean_log10": ie.get("mean_log10", ""),
            "ie_sd_log10": ie.get("sd_log10", ""),
        })
        if (i + 1) % 200 == 0:
            print(f"    stats {i + 1}/{len(site_years)}")
    comp_rows.sort(key=lambda r: (r["site_id"], r["season_year"]))
    write_csv(os.path.join(COMP_DIR, "bathing_water_compliance.csv"), COMPLIANCE_COLS, comp_rows)
    print(f"  wrote {len(comp_rows)} compliance rows")

    print("[4/4] fetching per-sample microbiology for near-cutoff 4-year windows ...")
    # Near-cutoff site-years are those classified Poor or Sufficient; each uses a
    # rolling 4-year sample window (year-3 .. year). Harvest the union of those
    # (point, year) sample sets once.
    want = set()
    for r in site_years:
        if r["season_year"] in DECISION_YEARS and r["classification_name"] in NEAR_CUTOFF:
            for wy in range(r["season_year"] - 3, r["season_year"] + 1):
                want.add((r["site_id"], wy))
    sample_rows = []
    want = sorted(want)
    for i, (sid, y) in enumerate(want):
        sample_rows.extend(samples_for(sid, y))
        if (i + 1) % 100 == 0:
            print(f"    sample windows {i + 1}/{len(want)} ({len(sample_rows)} samples)")
    sample_rows.sort(key=lambda r: (r["site_id"], r["sample_year"], r["sample_date_time"]))
    write_csv(os.path.join(SAMP_DIR, "bathing_water_samples.csv"), SAMPLE_COLS, sample_rows)
    print(f"  wrote {len(sample_rows)} sample rows across {len(want)} (point,year) windows")
    print("done.")


def write_csv(path, cols, rows):
    with open(path, "w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=cols, extrasaction="ignore")
        w.writeheader()
        for r in rows:
            w.writerow(r)


if __name__ == "__main__":
    main()
