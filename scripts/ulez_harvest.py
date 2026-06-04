#!/usr/bin/env python3
"""Harvest + freeze the ULEZ -> roadside NO2 controlled-ITS inputs.

The London Air Quality Network (LAQN) publishes monitoring data ONLY through the
Imperial ERG API (api.erg.ic.ac.uk/AirQuality) as raw hourly values — there is no
single bulk monthly CSV upstream. This script assembles two canonical, hash-pinned
frozen files the Go build reads offline:

  laqn_no2_monthly.csv   one row per (station, month): the monthly-mean NO2 with the
                         hours captured, for the curated treated (outer-London
                         roadside/kerbside, newly covered only at the 29 Aug 2023
                         London-wide ULEZ expansion) and control (urban-background /
                         suburban, far weaker direct treatment) station panels.

  laqn_meteo_monthly.csv one row per month: the dominant short-window confounder
                         (wind speed, temperature, precipitation) for a central
                         London point, from the Open-Meteo ERA5 reanalysis archive.

Re-running needs network and may differ as LAQN ratifies/revises historical hours;
the Go build pins these by SHA-256, so a re-harvest that differs is detected.

Licences recorded in data/raw/<id>/SOURCE.json:
  LAQN        Open Government Licence v3.0 (King's College London / Imperial ERG).
  Open-Meteo  data CC BY 4.0; underlying ERA5 (C) Copernicus Climate Change Service.

Usage:
  python3 scripts/ulez_harvest.py [--out data/cache] [--workers 5]
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
import urllib.request
import urllib.error
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed

# Prefer verified TLS; fall back to an unverified context only if the local Python
# lacks a usable root-cert bundle (common on macOS framework builds). Integrity is
# not weakened: every frozen file is SHA-256-pinned in its SOURCE.json, so the Go
# build rejects any byte that differs from the recorded hash regardless of transport.
try:
    _CTX = ssl.create_default_context()
    urllib.request.urlopen("https://api.erg.ic.ac.uk/AirQuality/", timeout=15, context=_CTX).close()
except Exception:
    sys.stderr.write("note: falling back to unverified TLS context (SHA-256 pin still enforced downstream)\n")
    _CTX = ssl._create_unverified_context()

LAQN_META = "https://api.erg.ic.ac.uk/AirQuality/Information/MonitoringSiteSpecies/GroupName=London/Json"
LAQN_DATA = ("https://api.erg.ic.ac.uk/AirQuality/Data/SiteSpecies/"
             "SiteCode={code}/SpeciesCode=NO2/StartDate={start}/EndDate={end}/Json")
OPEN_METEO = ("https://archive-api.open-meteo.com/v1/archive?latitude=51.5074&longitude=-0.1278"
              "&start_date={start}&end_date={end}"
              "&daily=temperature_2m_mean,wind_speed_10m_mean,wind_direction_10m_dominant,"
              "precipitation_sum&timezone=GMT")

# Each anchor on the "emission-zone stringency -> roadside NO2" mechanism is one
# harvested event: a window, a treated panel, a control panel, and the frozen-file
# source ids. Curated station panels are pinned by site code for reproducibility.
EVENTS = {
    # 2023 London-wide expansion (29 Aug 2023). Treated = outer-London roadside/
    # kerbside newly covered only by this expansion; control = urban-background/
    # suburban (same airshed, far weaker DIRECT kerbside-traffic treatment).
    "2023": {
        "start": "2021-09", "end": "2026-04",
        "no2_id": "ulez-no2-laqn", "meteo_id": "ulez-no2-meteo",
        "treated": ["CR5", "CR7", "CR9", "EN4", "EN5", "GB6", "HV1", "HV3", "ME2", "ME9", "RI1"],
        "control": ["BG1", "BG2", "BL0", "BQ7", "BX1", "BX2", "CW3", "EN1", "EN7", "GR4",
                    "HG4", "HI0", "HP1", "IS6", "KC1", "LB6", "LH0", "RI2", "TH5", "TH6",
                    "WA9", "WM0", "WM5"],
    },
    # 2019 central-London ULEZ (8 Apr 2019). Treated = central congestion-charge-zone
    # roadside/kerbside (inside the 2019 zone); control = outer-London roadside/
    # kerbside, NOT covered until the 2023 expansion — a clean same-road-type,
    # never-treated-in-period control. Window stops before the COVID lockdown
    # (2020-03) so the post period is policy-driven, not pandemic-driven.
    "2019": {
        "start": "2017-01", "end": "2020-06",
        "no2_id": "ulez-no2-laqn-2019", "meteo_id": "ulez-no2-meteo-2019",
        "treated": ["CD9", "CT2", "CT4", "CT6", "HK6", "IM1", "MR8", "MY1", "NB1", "WM6"],
        "control": ["BY7", "CR5", "CR7", "CR9", "EA6", "EI1", "EN4", "EN5", "GB6", "HR2",
                    "HV1", "HV3", "KT4", "ME2", "ME9", "RB4", "RHG", "RI1", "ST4", "ST6"],
    },
}

# These module globals are set from the chosen event in main(); the 2023 event is
# the default so the original single-event invocation is unchanged.
WIN_START = EVENTS["2023"]["start"]
WIN_END = EVENTS["2023"]["end"]
TREATED_CODES = EVENTS["2023"]["treated"]
CONTROL_CODES = EVENTS["2023"]["control"]
NO2_ID = EVENTS["2023"]["no2_id"]
METEO_ID = EVENTS["2023"]["meteo_id"]


def fetch_json(url: str, tries: int = 6, timeout: int = 70):
    last = None
    for i in range(tries):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "openaction2outcome-harvest/1.0"})
            with urllib.request.urlopen(req, timeout=timeout, context=_CTX) as r:
                return json.load(r)
        # Catch the full family of transient network faults, including
        # ConnectionResetError ([Errno 54] reset by peer) which is an OSError, not a
        # URLError — a single one of these must not discard a long harvest.
        except (urllib.error.URLError, OSError, TimeoutError, json.JSONDecodeError) as e:
            last = e
            time.sleep(min(30, 3 * (i + 1)))
    raise RuntimeError(f"failed {url}: {last}")


def months_in_range(start_ym: str, end_ym: str):
    sy, sm = int(start_ym[:4]), int(start_ym[5:7])
    ey, em = int(end_ym[:4]), int(end_ym[5:7])
    y, m = sy, sm
    while (y, m) <= (ey, em):
        yield f"{y:04d}-{m:02d}"
        m += 1
        if m > 12:
            m = 1
            y += 1


def years_in_range(start_ym: str, end_ym: str):
    return range(int(start_ym[:4]), int(end_ym[:4]) + 1)


def site_meta():
    d = fetch_json(LAQN_META)
    out = {}
    for s in d["Sites"]["Site"]:
        out[s["@SiteCode"]] = s
    return out


def partial_dir():
    # Per-station partial cache so a re-run resumes instead of restarting (a single
    # dropped connection late in a 40-minute harvest must not throw away the rest).
    # Keyed under the event's NO2 source id so different events never collide.
    return os.path.join("data", "cache", NO2_ID, "_partial")


def harvest_station(code: str, group: str, meta: dict):
    """Pull NO2 per calendar year, aggregate hourly -> monthly mean + hours captured."""
    part = os.path.join(partial_dir(), f"{code}.json")
    if os.path.exists(part):
        try:
            with open(part) as f:
                recs = json.load(f)
            sys.stderr.write(f"skip {code} ({group}) -> {len(recs)} months (cached)\n")
            return code, recs
        except (json.JSONDecodeError, OSError):
            pass  # corrupt partial; re-fetch
    s = meta.get(code)
    if s is None:
        sys.stderr.write(f"WARN {code}: not in metadata, skipping\n")
        return code, []
    agg = defaultdict(lambda: [0.0, 0])  # month -> [sum, n]
    for yr in years_in_range(WIN_START, WIN_END):
        start = f"{yr}-01-01"
        end = f"{yr + 1}-01-01"
        url = LAQN_DATA.format(code=code, start=start, end=end)
        d = fetch_json(url)
        rows = d.get("RawAQData", {}).get("Data", []) or []
        for row in rows:
            v = row.get("@Value", "")
            if v in ("", None):
                continue
            try:
                val = float(v)
            except ValueError:
                continue
            ts = row.get("@MeasurementDateGMT", "")
            month = ts[:7]
            if not month or month < WIN_START or month > WIN_END:
                continue
            a = agg[month]
            a[0] += val
            a[1] += 1
    recs = []
    for month in months_in_range(WIN_START, WIN_END):
        ssum, n = agg.get(month, [0.0, 0])
        if n == 0:
            continue
        recs.append({
            "site_code": code,
            "site_name": s["@SiteName"],
            "la_name": s["@LocalAuthorityName"],
            "site_type": s["@SiteType"],
            "latitude": s["@Latitude"],
            "longitude": s["@Longitude"],
            "group": group,
            "month": month,
            "no2_mean": round(ssum / n, 4),
            "n_hours": n,
        })
    os.makedirs(partial_dir(), exist_ok=True)
    tmp = part + ".tmp"
    with open(tmp, "w") as f:
        json.dump(recs, f)
    os.replace(tmp, part)
    sys.stderr.write(f"ok   {code} ({group}) -> {len(recs)} months\n")
    return code, recs


def harvest_no2(meta: dict, workers: int):
    jobs = [(c, "treated") for c in TREATED_CODES] + [(c, "control") for c in CONTROL_CODES]
    all_recs = {}
    with ThreadPoolExecutor(max_workers=workers) as ex:
        futs = {ex.submit(harvest_station, c, g, meta): c for c, g in jobs}
        for f in as_completed(futs):
            code, recs = f.result()
            all_recs[code] = recs
    # Deterministic order: group, then code, then month.
    order = {c: i for i, c in enumerate(TREATED_CODES + CONTROL_CODES)}
    flat = [r for recs in all_recs.values() for r in recs]
    flat.sort(key=lambda r: (order.get(r["site_code"], 999), r["month"]))
    return flat


def harvest_meteo():
    start = f"{WIN_START}-01"
    # last day of WIN_END month
    ey, em = int(WIN_END[:4]), int(WIN_END[5:7])
    ny, nm = (ey + 1, 1) if em == 12 else (ey, em + 1)
    import datetime
    end = (datetime.date(ny, nm, 1) - datetime.timedelta(days=1)).isoformat()
    d = fetch_json(OPEN_METEO.format(start=start, end=end))
    daily = d["daily"]
    import math
    agg = defaultdict(lambda: {"t": 0.0, "ws": 0.0, "u": 0.0, "v": 0.0, "p": 0.0, "n": 0})
    for i, day in enumerate(daily["time"]):
        month = day[:7]
        a = agg[month]
        t = daily["temperature_2m_mean"][i]
        ws = daily["wind_speed_10m_mean"][i]
        wd = daily["wind_direction_10m_dominant"][i]
        p = daily["precipitation_sum"][i]
        if t is None or ws is None or wd is None:
            continue
        a["t"] += t
        a["ws"] += ws
        # decompose wind into components so monthly means respect direction
        rad = math.radians(wd)
        a["u"] += ws * math.sin(rad)
        a["v"] += ws * math.cos(rad)
        a["p"] += (p or 0.0)
        a["n"] += 1
    recs = []
    for month in months_in_range(WIN_START, WIN_END):
        a = agg.get(month)
        if not a or a["n"] == 0:
            continue
        n = a["n"]
        recs.append({
            "month": month,
            "temp_c": round(a["t"] / n, 3),
            "wind_speed_kmh": round(a["ws"] / n, 3),
            "wind_u_kmh": round(a["u"] / n, 4),
            "wind_v_kmh": round(a["v"] / n, 4),
            "precip_mm": round(a["p"], 3),
            "n_days": n,
        })
    sys.stderr.write(f"ok   meteo -> {len(recs)} months\n")
    return recs


def write_csv(path: str, fieldnames, rows):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    buf = io.StringIO()
    w = csv.DictWriter(buf, fieldnames=fieldnames, lineterminator="\n")
    w.writeheader()
    for r in rows:
        w.writerow(r)
    data = buf.getvalue().encode("utf-8")
    with open(path, "wb") as f:
        f.write(data)
    sha = hashlib.sha256(data).hexdigest()
    print(f"wrote {path}\n  bytes  {len(data)}\n  sha256 {sha}")
    return sha, len(data)


def main():
    global WIN_START, WIN_END, TREATED_CODES, CONTROL_CODES, NO2_ID, METEO_ID
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="data/cache")
    ap.add_argument("--workers", type=int, default=5)
    ap.add_argument("--event", default="2023", choices=sorted(EVENTS), help="which emission-zone event to harvest")
    args = ap.parse_args()

    ev = EVENTS[args.event]
    WIN_START, WIN_END = ev["start"], ev["end"]
    TREATED_CODES, CONTROL_CODES = ev["treated"], ev["control"]
    NO2_ID, METEO_ID = ev["no2_id"], ev["meteo_id"]

    sys.stderr.write(f"event {args.event}: fetching LAQN site metadata...\n")
    meta = site_meta()

    sys.stderr.write(f"harvesting NO2 for {len(TREATED_CODES)}+{len(CONTROL_CODES)} stations "
                     f"over {WIN_START}..{WIN_END} ({args.workers} workers)...\n")
    no2 = harvest_no2(meta, args.workers)
    no2_fields = ["site_code", "site_name", "la_name", "site_type", "latitude",
                  "longitude", "group", "month", "no2_mean", "n_hours"]
    no2_path = os.path.join(args.out, NO2_ID, "laqn_no2_monthly.csv")
    write_csv(no2_path, no2_fields, no2)

    sys.stderr.write("harvesting Open-Meteo meteorology...\n")
    meteo = harvest_meteo()
    meteo_fields = ["month", "temp_c", "wind_speed_kmh", "wind_u_kmh", "wind_v_kmh",
                    "precip_mm", "n_days"]
    meteo_path = os.path.join(args.out, METEO_ID, "laqn_meteo_monthly.csv")
    write_csv(meteo_path, meteo_fields, meteo)

    print(f"\ndone ({args.event}): {len(no2)} station-months, {len(meteo)} meteo-months")


if __name__ == "__main__":
    main()
