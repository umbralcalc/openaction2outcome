#!/usr/bin/env python3
"""Harvest + freeze the Berlin Umweltzone (low-emission zone) → roadside-NO2 inputs.

The Berlin Umweltzone stage 2 (1 Jan 2010) required a green sticker (Euro-4 diesel /
Euro-1 petrol) to drive inside the S-Bahn Ring — a sharp standard-BAN tightening. We
read its effect as a controlled interrupted time series: monthly-mean NO2 at in-zone
traffic stations (treated) vs in-zone urban-background stations (control, far weaker
direct kerbside treatment), pre 2008-2009 (after stage 1, Jan 2008) vs post 2010-2011
— entirely pre-COVID.

Data source: the European Environment Agency Air Quality Download Service, historical
(AirBase, 2000-2012) dataset. EEA owns and publishes this under CC BY 4.0; the
underlying measurements were reported by Germany (UBA/Berlin Senate, BLUME network).
Each station's NO2 is one Azure-blob parquet (hourly, 1995-2012); we filter to valid
hours (Validity==1, Value>0; -999 is the missing sentinel) and aggregate to monthly.

Meteorology (the dominant short-window NO2 confounder) is Open-Meteo's ERA5 archive
for central Berlin, monthly — CC BY 4.0, contains modified Copernicus (ERA5) info.

NOTE on network sparsity: only TWO in-zone traffic NO2 stations have historical EEA
data covering 2008-2011 (Frankfurter Allee, Hardenbergplatz) — the pre-2013 Berlin
traffic network was small. This is the literature-standard Berlin LEZ panel; the thin
treated side widens the honest interval and is recorded as a limitation.

Usage: python3 scripts/berlin_lez_harvest.py [--out data/cache]
"""
from __future__ import annotations

import argparse
import csv
import hashlib
import io
import json
import math
import os
import ssl
import sys
import time
import urllib.request
import urllib.error
from collections import defaultdict

try:
    _CTX = ssl.create_default_context()
    urllib.request.urlopen("https://www.eea.europa.eu/", timeout=15, context=_CTX).close()
except Exception:
    sys.stderr.write("note: falling back to unverified TLS context (SHA-256 pin still enforced downstream)\n")
    _CTX = ssl._create_unverified_context()

EEA_URLS_API = "https://eeadmz1-downloads-api-appservice.azurewebsites.net/ParquetFile/urls"
NO2_VOCAB = "http://dd.eionet.europa.eu/vocabulary/aq/pollutant/8"
OPEN_METEO = ("https://archive-api.open-meteo.com/v1/archive?latitude=52.52&longitude=13.405"
              "&start_date={start}&end_date={end}"
              "&daily=temperature_2m_mean,wind_speed_10m_mean,wind_direction_10m_dominant,"
              "precipitation_sum&timezone=GMT")

WIN_START, WIN_END = "2008-01", "2011-12"

# Curated station panel, pinned by EEA/UBA station code (DEBExxx). Treated = in-zone
# (inside the S-Bahn Ring) traffic stations; control = in-zone urban-background.
# Each: (code, name, site_type, lat, lon, group).
STATIONS = [
    ("DEBE065", "Berlin Frankfurter Allee", "traffic",    52.5145, 13.4699, "treated"),
    ("DEBE067", "Berlin Hardenbergplatz",   "traffic",    52.5073, 13.3330, "treated"),
    ("DEBE010", "Berlin Wedding",           "background", 52.5430, 13.3491, "control"),
    ("DEBE018", "Berlin Schoeneberg",       "background", 52.4856, 13.3489, "control"),
    ("DEBE034", "Berlin Neukoelln",         "background", 52.4892, 13.4309, "control"),
    ("DEBE068", "Berlin Mitte",             "background", 52.5144, 13.4191, "control"),
]


def http(url, data=None, headers=None, tries=6, timeout=90):
    last = None
    for i in range(tries):
        try:
            req = urllib.request.Request(url, data=data, headers=headers or {})
            with urllib.request.urlopen(req, timeout=timeout, context=_CTX) as r:
                return r.read()
        except (urllib.error.URLError, OSError, TimeoutError) as e:
            last = e
            time.sleep(min(30, 3 * (i + 1)))
    raise RuntimeError(f"failed {url}: {last}")


def eea_url_map():
    """POST the EEA download API for all DE historical NO2 parquet URLs, return code->url."""
    body = json.dumps({
        "countries": ["DE"], "cities": [], "pollutants": [NO2_VOCAB],
        "dataset": 3, "source": "Api",
        "dateTimeStart": "2008-01-01T00:00:00Z", "dateTimeEnd": "2011-12-31T00:00:00Z",
    }).encode()
    raw = http(EEA_URLS_API, data=body,
               headers={"Content-Type": "application/json", "Accept": "*/*"}).decode("utf-8", "replace")
    m = {}
    for line in raw.splitlines():
        line = line.strip().strip('"')
        if not line.endswith(".parquet"):
            continue
        # filename: SPO.DE_DEBE065_NO2_dataGroup1.parquet
        fn = line.rsplit("/", 1)[-1]
        for part in fn.replace(".", "_").split("_"):
            if part.startswith("DEBE"):
                m.setdefault(part, line)
                break
    return m


def monthly_from_parquet(parquet_bytes):
    """Aggregate one station's hourly NO2 parquet to monthly mean + valid-hour count."""
    import pyarrow.parquet as pq
    buf = io.BytesIO(parquet_bytes)
    df = pq.read_table(buf, columns=["Start", "Value", "Validity"]).to_pandas()
    df = df[(df["Validity"] == 1) & (df["Value"] > 0)]
    starts = df["Start"].astype(str).str.slice(0, 7)
    agg = defaultdict(lambda: [0.0, 0])
    for ym, val in zip(starts.values, df["Value"].values):
        if ym < WIN_START or ym > WIN_END:
            continue
        a = agg[ym]
        a[0] += float(val)
        a[1] += 1
    return agg


def months_in_range(s, e):
    sy, sm = int(s[:4]), int(s[5:7])
    ey, em = int(e[:4]), int(e[5:7])
    y, mo = sy, sm
    while (y, mo) <= (ey, em):
        yield f"{y:04d}-{mo:02d}"
        mo += 1
        if mo > 12:
            mo = 1
            y += 1


def harvest_no2():
    sys.stderr.write("fetching EEA historical NO2 parquet URL list for DE...\n")
    urls = eea_url_map()
    rows = []
    for code, name, styp, lat, lon, group in STATIONS:
        url = urls.get(code)
        if not url:
            url = f"https://eeadmz1batchservice02.blob.core.windows.net/airquality-p-airbase/DE/SPO.DE_{code}_NO2_dataGroup1.parquet"
        sys.stderr.write(f"  {code} ({group}) <- {url.rsplit('/',1)[-1]}\n")
        agg = monthly_from_parquet(http(url))
        for ym in months_in_range(WIN_START, WIN_END):
            s, n = agg.get(ym, [0.0, 0])
            if n == 0:
                continue
            rows.append({
                "site_code": code, "site_name": name, "la_name": "Berlin",
                "site_type": styp, "latitude": lat, "longitude": lon,
                "group": group, "month": ym,
                "no2_mean": round(s / n, 4), "n_hours": n,
            })
    rows.sort(key=lambda r: (r["group"] != "treated", r["site_code"], r["month"]))
    return rows


def harvest_meteo():
    end = "2011-12-31"
    d = json.loads(http(OPEN_METEO.format(start=WIN_START + "-01", end=end)))
    daily = d["daily"]
    agg = defaultdict(lambda: {"t": 0.0, "ws": 0.0, "u": 0.0, "v": 0.0, "p": 0.0, "n": 0})
    for i, day in enumerate(daily["time"]):
        a = agg[day[:7]]
        t, ws, wd, p = (daily["temperature_2m_mean"][i], daily["wind_speed_10m_mean"][i],
                        daily["wind_direction_10m_dominant"][i], daily["precipitation_sum"][i])
        if t is None or ws is None or wd is None:
            continue
        rad = math.radians(wd)
        a["t"] += t; a["ws"] += ws; a["u"] += ws * math.sin(rad); a["v"] += ws * math.cos(rad)
        a["p"] += (p or 0.0); a["n"] += 1
    out = []
    for ym in months_in_range(WIN_START, WIN_END):
        a = agg.get(ym)
        if not a or a["n"] == 0:
            continue
        n = a["n"]
        out.append({"month": ym, "temp_c": round(a["t"] / n, 3), "wind_speed_kmh": round(a["ws"] / n, 3),
                    "wind_u_kmh": round(a["u"] / n, 4), "wind_v_kmh": round(a["v"] / n, 4),
                    "precip_mm": round(a["p"], 3), "n_days": n})
    return out


def write_csv(path, fields, rows):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    buf = io.StringIO()
    w = csv.DictWriter(buf, fieldnames=fields, lineterminator="\n")
    w.writeheader()
    for r in rows:
        w.writerow(r)
    data = buf.getvalue().encode("utf-8")
    with open(path, "wb") as f:
        f.write(data)
    print(f"wrote {path}\n  bytes  {len(data)}\n  sha256 {hashlib.sha256(data).hexdigest()}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="data/cache")
    args = ap.parse_args()
    no2 = harvest_no2()
    write_csv(os.path.join(args.out, "berlin-lez-no2", "berlin_no2_monthly.csv"),
              ["site_code", "site_name", "la_name", "site_type", "latitude", "longitude",
               "group", "month", "no2_mean", "n_hours"], no2)
    sys.stderr.write("harvesting Open-Meteo (Berlin)...\n")
    write_csv(os.path.join(args.out, "berlin-lez-meteo", "berlin_meteo_monthly.csv"),
              ["month", "temp_c", "wind_speed_kmh", "wind_u_kmh", "wind_v_kmh", "precip_mm", "n_days"],
              harvest_meteo())
    print(f"\ndone: {len(no2)} station-months")


if __name__ == "__main__":
    main()
