#!/usr/bin/env python3
"""Harvest + freeze the US flavour-ban → smoking DiD outcome panel.

Adult current-smoking prevalence by US state × year (2011-2024) from the CDC
Behavioral Risk Factor Surveillance System, via the data.cdc.gov Socrata dataset
dttw-5yxu ("BRFSS Prevalence Data, 2011 to present"). We take the calculated
"Current Smoker Status" = "Yes", break_out "Overall", with its 95% confidence
limits (→ a per-cell standard error for DiD inference) and sample size.

Output one compact, hash-pinned CSV with the SAME column schema as the Canadian
menthol panel (province=state abbreviation), so internal/ingest.LoadSmokingPanel
reads both. Licence: U.S. Government work, public domain (17 U.S.C. § 105). NCHS/
BRFSS carry a "statistical reporting and analysis only; no linkage to re-identify
individuals" use restriction — recorded in SOURCE.json; aggregate DiD use is fine.

Usage: python3 scripts/usflavour_harvest.py [--out data/cache]
"""
from __future__ import annotations

import argparse
import csv
import hashlib
import io
import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

try:
    _CTX = ssl.create_default_context()
    urllib.request.urlopen("https://data.cdc.gov/", timeout=20, context=_CTX).close()
except Exception:
    sys.stderr.write("note: falling back to unverified TLS context (SHA-256 pin still enforced downstream)\n")
    _CTX = ssl._create_unverified_context()

RESOURCE = "https://data.cdc.gov/resource/dttw-5yxu.json"

# 50 states + DC (BRFSS state abbreviations); exclude national US and territories.
STATES = {
    "AL", "AK", "AZ", "AR", "CA", "CO", "CT", "DE", "DC", "FL", "GA", "HI", "ID", "IL",
    "IN", "IA", "KS", "KY", "LA", "ME", "MD", "MA", "MI", "MN", "MS", "MO", "MT", "NE",
    "NV", "NH", "NJ", "NM", "NY", "NC", "ND", "OH", "OK", "OR", "PA", "RI", "SC", "SD",
    "TN", "TX", "UT", "VT", "VA", "WA", "WV", "WI", "WY",
}


def fetch(url, tries=6):
    last = None
    for i in range(tries):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "openaction2outcome-harvest/1.0"})
            with urllib.request.urlopen(req, timeout=90, context=_CTX) as r:
                return json.load(r)
        except (urllib.error.URLError, OSError, TimeoutError, json.JSONDecodeError) as e:
            last = e
            sys.stderr.write(f"  retry {i+1}/{tries}: {e}\n")
            time.sleep(min(30, 4 * (i + 1)))
    raise RuntimeError(f"failed {url}: {last}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="data/cache")
    args = ap.parse_args()

    where = ("topic='Current Smoker Status' AND response='Yes' AND break_out='Overall' "
             "AND data_value IS NOT NULL")
    sel = "year,locationabbr,data_value,confidence_limit_low,confidence_limit_high,sample_size"
    url = (f"{RESOURCE}?$select={urllib.parse.quote(sel)}"
           f"&$where={urllib.parse.quote(where)}&$limit=50000&$order=locationabbr,year")
    sys.stderr.write("querying BRFSS current-smoking (data.cdc.gov dttw-5yxu)...\n")
    data = fetch(url)

    rows = []
    for r in data:
        st = r.get("locationabbr", "")
        if st not in STATES:
            continue
        pct = r.get("data_value")
        if pct in (None, ""):
            continue
        rows.append({
            "province": st,
            "year": int(r["year"][:4]),
            "smoking_pct": pct,
            "smoking_lo": r.get("confidence_limit_low", ""),
            "smoking_hi": r.get("confidence_limit_high", ""),
            "sample_size": r.get("sample_size", ""),
            "source_table": "BRFSS-dttw-5yxu",
        })
    rows.sort(key=lambda r: (r["province"], r["year"]))

    buf = io.StringIO()
    w = csv.DictWriter(buf, fieldnames=["province", "year", "smoking_pct", "smoking_lo",
                                        "smoking_hi", "sample_size", "source_table"],
                       lineterminator="\n")
    w.writeheader()
    for r in rows:
        w.writerow(r)
    blob = buf.getvalue().encode("utf-8")

    path = os.path.join(args.out, "us-flavour-smoking", "brfss_smoking_panel.csv")
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(blob)
    print(f"wrote {path}\n  rows   {len(rows)}\n  bytes  {len(blob)}\n  sha256 {hashlib.sha256(blob).hexdigest()}")
    states = sorted({r["province"] for r in rows})
    yrs = sorted({r["year"] for r in rows})
    print(f"  states {len(states)} (incl DC), years {yrs[0]}..{yrs[-1]}")


if __name__ == "__main__":
    main()
