#!/usr/bin/env python3
"""One-time CQC inspection extract for the fuzzy SHMI mark.

Reads the CQC API subscription key from .cqc_api_key (gitignored), looks up each
NHS trust appearing in the SHMI decision windows directly by its ODS code, and
writes a frozen, deterministic CSV of dated CQC report (inspection) events:

    provider_ods, provider_name, report_date, report_type

The key is used only here. The output CSV is the canonical frozen input
(hash-pinned, committed as a pointer, mirrored to object storage); the build
reads it without the key. Re-running needs a key and may differ as CQC updates.
"""
import csv, io, json, os, subprocess, sys, time, zipfile

KEY = open(".cqc_api_key").read().strip()
ZIP = "data/cache/shmi-historical-oct24-sep25/SHMI_data_Oct24-Sep25.zip"
DECISION_WINDOWS = {"APR18_MAR19", "APR22_MAR23", "APR23_MAR24"}
OUT_DIR = "data/cache/cqc-inspections-2026-05-30"
API = "https://api.service.cqc.org.uk/public/v1/providers/"


def trust_codes():
    codes = set()
    with zipfile.ZipFile(ZIP) as z:
        name = next(n for n in z.namelist()
                    if "Historical_trust_level_SHMI_data" in n and n.endswith(".csv"))
        with z.open(name) as f:
            for row in csv.DictReader(io.TextIOWrapper(f, encoding="utf-8-sig")):
                if row["TIME_PERIOD"] in DECISION_WINDOWS:
                    codes.add(row["PROVIDER_CODE"])
    return sorted(codes)


def fetch(code):
    r = subprocess.run(
        ["curl", "-sS", "--max-time", "30",
         "-H", f"Ocp-Apim-Subscription-Key: {KEY}", API + code],
        capture_output=True, text=True)
    return r.stdout


def main():
    codes = trust_codes()
    print(f"{len(codes)} trust codes across {sorted(DECISION_WINDOWS)}")
    rows, missing = [], []
    for i, code in enumerate(codes):
        try:
            d = json.loads(fetch(code))
        except Exception:
            missing.append(code); continue
        if "name" not in d:  # 404 / error payload
            missing.append(code); continue
        name = d.get("name", "")
        for rep in d.get("reports", []) or []:
            rd = rep.get("reportDate")
            if rd:
                rows.append((code, name, rd, rep.get("reportType", "")))
        time.sleep(0.15)
        if (i + 1) % 25 == 0:
            print(f"  {i + 1}/{len(codes)} ...")
    print(f"resolved {len(codes) - len(missing)} trusts; {len(missing)} unresolved: {missing[:12]}")
    os.makedirs(OUT_DIR, exist_ok=True)
    path = os.path.join(OUT_DIR, "cqc_inspections.csv")
    with open(path, "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["provider_ods", "provider_name", "report_date", "report_type"])
        for r in sorted(rows):
            w.writerow(r)
    print(f"wrote {len(rows)} report events -> {path}")


if __name__ == "__main__":
    main()
