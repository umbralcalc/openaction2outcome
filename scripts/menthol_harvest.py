#!/usr/bin/env python3
"""Harvest + freeze the Canadian menthol-ban → smoking DiD outcome panel.

Adult (12+) current-smoking prevalence by province × year, from the Canadian
Community Health Survey via two Statistics Canada tables stitched at the 2015 CCHS
redesign (a common shock the DiD differences out):

  13-10-0451  Health indicators (CCHS), 2003–2014  (pre-redesign)
  13-10-0096  Health characteristics, annual estimates, 2015+  (post-redesign)

We take "Current smoker, daily or occasional", "Percent", "Total, 12 years and
over", "Both sexes" — the only total available in BOTH tables — plus its 95% CI
(→ a per-cell standard error for DiD inference). Output one compact, hash-pinned
CSV: one row per (province, year) with the smoking percent and CI.

Licence: Statistics Canada Open Licence (reproduce/adapt/redistribute derived data
with attribution). Attribution belongs in SOURCE.json and the mark provenance:
"Adapted from Statistics Canada, tables 13-10-0451 and 13-10-0096."

Usage: python3 scripts/menthol_harvest.py [--out data/cache]
"""
from __future__ import annotations

import argparse
import csv
import hashlib
import io
import os
import ssl
import sys
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import zipfile

try:
    _CTX = ssl.create_default_context()
    urllib.request.urlopen("https://www150.statcan.gc.ca/", timeout=20, context=_CTX).close()
except Exception:
    sys.stderr.write("note: falling back to unverified TLS context (SHA-256 pin still enforced downstream)\n")
    _CTX = ssl._create_unverified_context()

TABLES = {
    "pre":  ("13100451", "https://www150.statcan.gc.ca/n1/tbl/csv/13100451-eng.zip", range(2007, 2015)),
    "post": ("13100096", "https://www150.statcan.gc.ca/n1/tbl/csv/13100096-eng.zip", range(2015, 2020)),
}

PROVINCES = {
    "Newfoundland and Labrador": "NL", "Prince Edward Island": "PE", "Nova Scotia": "NS",
    "New Brunswick": "NB", "Quebec": "QC", "Ontario": "ON", "Manitoba": "MB",
    "Saskatchewan": "SK", "Alberta": "AB", "British Columbia": "BC",
}
# Ontario has no plain province row in the 2003–2014 table; its province total is the
# "by Health Unit" rollup. Other provinces use their plain name in both tables.
GEO_OVERRIDE_PRE = {"Ontario": "Ontario by Health Unit"}

INDICATOR = "Current smoker, daily or occasional"
AGE = "Total, 12 years and over"


def download_csv(url: str, pid: str, tries: int = 5) -> str:
    # The 2003-2014 table is ~140 MB; Python's urllib repeatedly times out on it
    # here, but curl streams it reliably (resumable, system trust store). Shell out
    # to curl with retries, then read the CSV out of the zip.
    last = None
    for i in range(tries):
        with tempfile.NamedTemporaryFile(suffix=".zip", delete=False) as tf:
            zpath = tf.name
        try:
            subprocess.run(
                ["curl", "-fsSL", "--retry", "3", "--retry-delay", "5", "--max-time", "600",
                 "-o", zpath, url],
                check=True,
            )
            return zipfile.ZipFile(zpath).read(f"{pid}.csv").decode("utf-8-sig")
        except (subprocess.CalledProcessError, zipfile.BadZipFile, OSError) as e:
            last = e
            sys.stderr.write(f"  retry {i+1}/{tries} ({pid}): {e}\n")
            time.sleep(min(30, 5 * (i + 1)))
        finally:
            try:
                os.unlink(zpath)
            except OSError:
                pass
    raise RuntimeError(f"failed to download {pid}: {last}")


def extract(text: str, years, geo_for):
    """Return {(prov_code, year): {pct, lo, hi}} for the smoking total."""
    want = {p: {} for p in PROVINCES.values()}
    geo_to_prov = {geo_for(p): code for p, code in PROVINCES.items()}
    rdr = csv.DictReader(io.StringIO(text))
    for row in rdr:
        if row["Indicators"] != INDICATOR or row["Age group"] != AGE or row["Sex"] != "Both sexes":
            continue
        code = geo_to_prov.get(row["GEO"])
        if code is None:
            continue
        try:
            yr = int(row["REF_DATE"][:4])
        except ValueError:
            continue
        if yr not in years:
            continue
        ch, val = row["Characteristics"], row["VALUE"].strip()
        if val == "":
            continue
        cell = want[code].setdefault(yr, {})
        if ch == "Percent":
            cell["pct"] = val
        elif ch == "Low 95% confidence interval, percent":
            cell["lo"] = val
        elif ch == "High 95% confidence interval, percent":
            cell["hi"] = val
    return want


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="data/cache")
    args = ap.parse_args()

    rows = []
    for key, (pid, url, years) in TABLES.items():
        sys.stderr.write(f"downloading StatCan table {pid} ({key})...\n")
        text = download_csv(url, pid)
        geo_for = (lambda p: GEO_OVERRIDE_PRE.get(p, p)) if key == "pre" else (lambda p: p)
        data = extract(text, set(years), geo_for)
        for code in PROVINCES.values():
            for yr in sorted(data[code]):
                c = data[code][yr]
                if "pct" not in c:
                    continue
                rows.append({
                    "province": code, "year": yr, "smoking_pct": c["pct"],
                    "smoking_lo": c.get("lo", ""), "smoking_hi": c.get("hi", ""),
                    "source_table": pid,
                })
        got = sum(1 for code in PROVINCES.values() for yr in data[code] if "pct" in data[code][yr])
        sys.stderr.write(f"  {pid}: {got} province-years\n")

    rows.sort(key=lambda r: (r["province"], r["year"]))
    buf = io.StringIO()
    w = csv.DictWriter(buf, fieldnames=["province", "year", "smoking_pct", "smoking_lo", "smoking_hi", "source_table"],
                       lineterminator="\n")
    w.writeheader()
    for r in rows:
        w.writerow(r)
    data = buf.getvalue().encode("utf-8")

    path = os.path.join(args.out, "ca-menthol-smoking", "statcan_smoking_panel.csv")
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(data)
    print(f"wrote {path}\n  rows   {len(rows)}\n  bytes  {len(data)}\n  sha256 {hashlib.sha256(data).hexdigest()}")
    # coverage sanity
    provs = {r["province"] for r in rows}
    yrs = sorted({r["year"] for r in rows})
    print(f"  provinces {len(provs)}/10, years {yrs[0]}..{yrs[-1]}")


if __name__ == "__main__":
    main()
