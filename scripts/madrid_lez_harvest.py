#!/usr/bin/env python3
"""Harvest + freeze the Madrid Central (low-emission zone) → roadside/in-zone-NO2 inputs.

Madrid Central came into effect on 30 November 2018: a ~472-ha access-restriction LEZ
covering the Centro district, barring non-resident combustion vehicles from the city
core (an access BAN, not a charge — the same regime family as the Berlin Umweltzone).
We read its effect as a controlled interrupted time series: monthly-mean NO2 at the
in-zone municipal station (treated) vs municipal urban/suburban-background stations
away from the zone boundary (control, sharing Madrid's regional/meteorological trend
but outside the restriction). Pre 2017-01..2018-11 vs post 2018-12..2019-06 — kept
short, before the July 2019 sanction moratorium and entirely pre-COVID.

Data source: the European Environment Agency Air Quality Download Service, VERIFIED
(E1a, 2013+) dataset. EEA owns and publishes this under CC BY 4.0; the underlying
measurements were reported by Spain (Madrid City Council municipal network, the same
SIVPICA stations the published Madrid Central evaluations use). Each station's NO2 is
one Azure-blob parquet (hourly); we filter to valid hours (Validity==1, Value>0) and
aggregate to monthly means.

Meteorology (the dominant short-window NO2 confounder) is Open-Meteo's ERA5 archive
for central Madrid, monthly — CC BY 4.0, contains modified Copernicus (ERA5) info.

TREATED = the in-zone monitor used in the literature:
  28079035 Plaza del Carmen (Centro, urban background, INSIDE Madrid Central).
CONTROL = municipal urban/suburban-background stations away from the zone boundary.
Traffic stations and near-boundary stations (Plaza de España, Escuelas Aguirre, Cuatro
Caminos, Retiro) are EXCLUDED: the published work finds positive boundary spillover, so
near-zone stations are contaminated controls, not clean counterfactuals.

Usage: python3 scripts/madrid_lez_harvest.py [--out data/cache]
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
# E1a (verified) blob layout for Spain: SP_<nationalcode>_8_8.parquet, hourly NO2.
EEA_BLOB = "https://eeadmz1batchservice02.blob.core.windows.net/airquality-p-e1a/ES/SP_{code}_8_8.parquet"
OPEN_METEO = ("https://archive-api.open-meteo.com/v1/archive?latitude=40.4168&longitude=-3.7038"
              "&start_date={start}&end_date={end}"
              "&daily=temperature_2m_mean,wind_speed_10m_mean,wind_direction_10m_dominant,"
              "precipitation_sum&timezone=GMT")

WIN_START, WIN_END = "2017-01", "2019-12"

# Curated Madrid municipal-network panel, pinned by Spanish national station code
# (28079xxx). site_type and coordinates are the Madrid City Council classification.
# Treated = in-zone (Madrid Central / Centro) station; control = urban/suburban
# background away from the zone, excluding traffic and near-boundary stations.
# Each: (code, name, site_type, lat, lon, group).
STATIONS = [
    ("28079035", "Plaza del Carmen",      "background", 40.41924, -3.70256, "treated"),
    ("28079017", "Villaverde",            "background", 40.34715, -3.71325, "control"),
    ("28079018", "Farolillo",             "background", 40.39478, -3.73172, "control"),
    ("28079024", "Casa de Campo",         "background", 40.41936, -3.74757, "control"),
    ("28079027", "Barajas Pueblo",        "background", 40.47694, -3.58003, "control"),
    ("28079036", "Moratalaz",             "background", 40.40794, -3.64531, "control"),
    ("28079039", "Barrio del Pilar",      "background", 40.47846, -3.71154, "control"),
    ("28079040", "Vallecas",              "background", 40.38805, -3.65165, "control"),
    ("28079047", "Mendez Alvaro",         "background", 40.39800, -3.68684, "control"),
    ("28079054", "Ensanche de Vallecas",  "background", 40.37288, -3.61204, "control"),
    ("28079055", "Urbanizacion Embajada", "background", 40.46243, -3.58063, "control"),
    ("28079057", "Sanchinarro",           "background", 40.49424, -3.66060, "control"),
    ("28079058", "El Pardo",              "background", 40.51806, -3.77463, "control"),
    ("28079059", "Juan Carlos I",         "background", 40.46554, -3.61636, "control"),
    ("28079060", "Tres Olivos",           "background", 40.50055, -3.68961, "control"),
]


def http(url, data=None, headers=None, tries=6, timeout=120):
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
    """POST the EEA download API for all ES verified NO2 parquet URLs, return code->url."""
    body = json.dumps({
        "countries": ["ES"], "cities": [], "pollutants": [NO2_VOCAB],
        "dataset": 2, "source": "Api",
        "dateTimeStart": WIN_START + "-01T00:00:00Z", "dateTimeEnd": WIN_END + "-31T00:00:00Z",
    }).encode()
    raw = http(EEA_URLS_API, data=body,
               headers={"Content-Type": "application/json", "Accept": "*/*"}).decode("utf-8", "replace")
    m = {}
    for line in raw.splitlines():
        line = line.strip().strip('"')
        if not line.endswith(".parquet"):
            continue
        # filename: SP_28079035_8_8.parquet
        fn = line.rsplit("/", 1)[-1]
        parts = fn.split("_")
        if len(parts) >= 2 and parts[1].isdigit():
            m.setdefault(parts[1], line)
    return m


def monthly_from_parquet(parquet_bytes):
    """Aggregate one station's hourly NO2 parquet to monthly mean + valid-hour count."""
    import pyarrow.parquet as pq
    buf = io.BytesIO(parquet_bytes)
    df = pq.read_table(buf, columns=["Start", "Value", "Validity"]).to_pandas()
    df = df[(df["Validity"] == 1) & (df["Value"].astype(float) > 0)]
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
    sys.stderr.write("fetching EEA verified (E1a) NO2 parquet URL list for ES...\n")
    try:
        urls = eea_url_map()
    except Exception as e:
        sys.stderr.write(f"  url-list API failed ({e}); using constructed blob URLs\n")
        urls = {}
    rows = []
    for code, name, styp, lat, lon, group in STATIONS:
        url = urls.get(code) or EEA_BLOB.format(code=code)
        sys.stderr.write(f"  {code} ({group}) <- {url.rsplit('/',1)[-1]}\n")
        agg = monthly_from_parquet(http(url))
        for ym in months_in_range(WIN_START, WIN_END):
            s, n = agg.get(ym, [0.0, 0])
            if n == 0:
                continue
            rows.append({
                "site_code": code, "site_name": name, "la_name": "Madrid",
                "site_type": styp, "latitude": lat, "longitude": lon,
                "group": group, "month": ym,
                "no2_mean": round(s / n, 4), "n_hours": n,
            })
    rows.sort(key=lambda r: (r["group"] != "treated", r["site_code"], r["month"]))
    return rows


def harvest_meteo():
    end = WIN_END + "-31"
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
    write_csv(os.path.join(args.out, "madrid-lez-no2", "madrid_no2_monthly.csv"),
              ["site_code", "site_name", "la_name", "site_type", "latitude", "longitude",
               "group", "month", "no2_mean", "n_hours"], no2)
    sys.stderr.write("harvesting Open-Meteo (Madrid)...\n")
    write_csv(os.path.join(args.out, "madrid-lez-meteo", "madrid_meteo_monthly.csv"),
              ["month", "temp_c", "wind_speed_kmh", "wind_u_kmh", "wind_v_kmh", "precip_mm", "n_days"],
              harvest_meteo())
    print(f"\ndone: {len(no2)} station-months")


if __name__ == "__main__":
    main()
