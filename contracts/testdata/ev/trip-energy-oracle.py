#!/usr/bin/env python3
"""Independent oracle for contracts/testdata/ev/trip-energy-v1.csv (MAP-26).

Written with Python Fractions only, not derived from the Kotlin implementation. Each case lists
inputs and the hand-reasoned expectation; `--check` recomputes every row and fails on any drift, so
the CSV cannot be silently rewritten to match a defect. Run: python3 trip-energy-oracle.py --check
"""
import csv
import math
import sys
from fractions import Fraction
from pathlib import Path

MAX_ENERGY_WH = 10**12
MAX_DISTANCE_M = 10**12
FIELDS = ["id", "operation", "capacity_wh", "soc_bp", "reserve_bp", "usable_permille", "cons_milli",
          "distance_m", "margin_permille", "power_w", "energy_wh", "sessions", "expected"]

def floor(x): return math.floor(x)
def ceil(x): return math.ceil(x)

def bad(v, lo, hi): return v is None or not (lo <= v <= hi)

def usable(capacity, soc, reserve, portion):
    if capacity is None: return "unknown:capacity-missing"
    if soc is None: return "unknown:soc-missing"
    if reserve is None: return "unknown:reserve-missing"
    if bad(capacity, 1, MAX_ENERGY_WH) or bad(soc, 0, 10000) or bad(reserve, 0, 10000): return "unknown:input-out-of-range"
    if portion is not None and bad(portion, 1, 1000): return "unknown:input-out-of-range"
    stored = Fraction(capacity) * max(soc - reserve, 0) / 10000
    if portion is not None: stored = stored * portion / 1000
    return floor(stored)

def eff_cons(cons, margin):
    if cons is None: return "unknown:consumption-missing"
    if bad(cons, 1, 10**9): return "unknown:consumption-nonpositive" if cons <= 0 else "unknown:input-out-of-range"
    if margin is None: margin = 0
    if bad(margin, 0, 1000): return "unknown:margin-invalid"
    return Fraction(cons) * (1000 + margin) / 1000

def expect(row):
    op = row["operation"]
    g = lambda k: None if row[k] == "" else int(row[k])
    if op == "USABLE":
        r = usable(g("capacity_wh"), g("soc_bp"), g("reserve_bp"), g("usable_permille"))
    elif op == "SEGMENT":
        c = eff_cons(g("cons_milli"), g("margin_permille"))
        d = g("distance_m")
        if isinstance(c, str): r = c
        elif d is None: r = "unknown:distance-missing"
        elif bad(d, 0, MAX_DISTANCE_M): r = "unknown:negative-input" if d < 0 else "unknown:input-out-of-range"
        else:
            r = ceil(Fraction(d) * c / 10**6)
            if r > MAX_ENERGY_WH: r = "unknown:out-of-bounds"
    elif op == "REACH":
        u = usable(g("capacity_wh"), g("soc_bp"), g("reserve_bp"), g("usable_permille"))
        c = eff_cons(g("cons_milli"), g("margin_permille"))
        if isinstance(u, str): r = u
        elif isinstance(c, str): r = c
        else:
            r = floor(Fraction(u) * 10**6 / c)
            if r > MAX_DISTANCE_M: r = "unknown:out-of-bounds"
    elif op == "ARRIVAL":
        cap, soc = g("capacity_wh"), g("soc_bp")
        c = eff_cons(g("cons_milli"), g("margin_permille"))
        d = g("distance_m")
        if cap is None: r = "unknown:capacity-missing"
        elif soc is None: r = "unknown:soc-missing"
        elif isinstance(c, str): r = c
        elif d is None: r = "unknown:distance-missing"
        elif bad(cap, 1, MAX_ENERGY_WH) or bad(soc, 0, 10000): r = "unknown:input-out-of-range"
        elif bad(d, 0, MAX_DISTANCE_M): r = "unknown:negative-input" if d < 0 else "unknown:input-out-of-range"
        else:
            stored = Fraction(cap) * soc / 10000
            need = Fraction(ceil(Fraction(d) * c / 10**6))
            r = "unknown:energy-exceeds-battery" if need > stored else floor((stored - need) * 10000 / cap)
    elif op == "CHARGE_LB":
        e, p = g("energy_wh"), g("power_w")
        if e is None: r = "unknown:energy-missing"
        elif p is None: r = "unknown:power-missing"
        elif e < 0: r = "unknown:negative-input"
        elif p <= 0: r = "unknown:power-nonpositive"
        elif e > MAX_ENERGY_WH: r = "unknown:input-out-of-range"
        else: r = floor(Fraction(e) * 3600 / p)
    elif op == "CALIBRATE":
        sessions = [tuple(int(x) for x in s.split(":")) for s in row["sessions"].split(";") if s]
        if not sessions: r = "unknown:calibration-insufficient"
        elif any(w <= 0 or m <= 0 for w, m in sessions): r = "unknown:session-invalid"
        else:
            wh, m = sum(s[0] for s in sessions), sum(s[1] for s in sessions)
            r = "unknown:calibration-insufficient" if m < 20000 else ceil(Fraction(wh) * 10**6 / m)
    else:
        raise SystemExit("unknown operation " + op)
    return str(r)

ROWS = [
    # id, op, capacity, soc, reserve, portion, cons, distance, margin, power, energy, sessions
    ("U01", "USABLE", 60000, 8000, 1000, None, None, None, None, None, None, ""),          # 60000*7000/10000 = 42000
    ("U02", "USABLE", 60000, 1000, 1000, None, None, None, None, None, None, ""),          # at reserve -> 0
    ("U03", "USABLE", 60000, 500, 1000, None, None, None, None, None, None, ""),           # below reserve -> 0, never negative
    ("U04", "USABLE", 60000, 10000, 0, None, None, None, None, None, None, ""),            # full -> 60000
    ("U05", "USABLE", 12345, 3333, 0, None, None, None, None, None, None, ""),             # 12345*3333/10000 = 4114.6 -> 4114
    ("U06", "USABLE", 15000, 5000, 0, 800, None, None, None, None, None, ""),              # PHEV: 7500 * 0.8 = 6000
    ("U07", "USABLE", None, 5000, 0, None, None, None, None, None, None, ""),
    ("U08", "USABLE", 60000, None, 0, None, None, None, None, None, None, ""),
    ("U09", "USABLE", 60000, 5000, None, None, None, None, None, None, None, ""),
    ("U10", "USABLE", 60000, 10001, 0, None, None, None, None, None, None, ""),
    ("U11", "USABLE", 0, 5000, 0, None, None, None, None, None, None, ""),
    ("U12", "USABLE", 10**12, 10000, 0, None, None, None, None, None, None, ""),           # ceiling accepted
    ("U13", "USABLE", 10**12 + 1, 10000, 0, None, None, None, None, None, None, ""),
    ("S01", "SEGMENT", None, None, None, None, 150000, 100000, 0, None, None, ""),         # 100 km @150 -> 15000
    ("S02", "SEGMENT", None, None, None, None, 150000, 100000, 100, None, None, ""),       # +10% -> 16500
    ("S03", "SEGMENT", None, None, None, None, 150000, 12345, 0, None, None, ""),          # 1851.75 -> ceil 1852
    ("S04", "SEGMENT", None, None, None, None, 150000, 0, 0, None, None, ""),              # 0 m -> 0
    ("S05", "SEGMENT", None, None, None, None, 150000, 1, 0, None, None, ""),              # 0.15 Wh -> ceil 1
    ("S06", "SEGMENT", None, None, None, None, 150000, -1, 0, None, None, ""),
    ("S07", "SEGMENT", None, None, None, None, None, 1000, 0, None, None, ""),
    ("S08", "SEGMENT", None, None, None, None, 0, 1000, 0, None, None, ""),
    ("S09", "SEGMENT", None, None, None, None, -5, 1000, 0, None, None, ""),
    ("S10", "SEGMENT", None, None, None, None, 150000, 1000, -1, None, None, ""),
    ("S11", "SEGMENT", None, None, None, None, 150000, 1000, 1001, None, None, ""),
    ("S12", "SEGMENT", None, None, None, None, 10**9, 10**12, 0, None, None, ""),          # 1e12 Wh*km... -> above energy ceiling
    ("S13", "SEGMENT", None, None, None, None, 150000, None, 0, None, None, ""),
    ("R01", "REACH", 60000, 8000, 1000, None, 150000, None, 0, None, None, ""),            # 42000 Wh / 150 = 280 km
    ("R02", "REACH", 60000, 8000, 1000, None, 150000, None, 100, None, None, ""),          # /165 = 254.545 km -> 254545
    ("R03", "REACH", 60000, 1000, 1000, None, 150000, None, 0, None, None, ""),            # nothing usable -> 0
    ("R04", "REACH", 15000, 5000, 0, 800, 200000, None, 0, None, None, ""),                # 6000 Wh /200 = 30 km
    ("R05", "REACH", 60000, 8000, 1000, None, None, None, 0, None, None, ""),
    ("R06", "REACH", 60000, 8000, 1000, None, 0, None, 0, None, None, ""),
    ("R07", "REACH", 10**12, 10000, 0, None, 1, None, 0, None, None, ""),                  # 1e18 m -> above distance ceiling
    ("R08", "REACH", 60000, 8000, None, None, 150000, None, 0, None, None, ""),
    ("A01", "ARRIVAL", 60000, 8000, None, None, 150000, 100000, 0, None, None, ""),        # 48000-15000 = 33000 -> 5500 bp
    ("A02", "ARRIVAL", 60000, 8000, None, None, 150000, 12345, 0, None, None, ""),         # 48000-1852 = 46148 -> 7691.33 -> 7691
    ("A03", "ARRIVAL", 60000, 8000, None, None, 150000, 320000, 0, None, None, ""),        # needs 48000 -> 0 bp exactly
    ("A04", "ARRIVAL", 60000, 8000, None, None, 150000, 320001, 0, None, None, ""),        # needs 48001 > 48000
    ("A05", "ARRIVAL", 60000, 8000, None, None, 150000, 100000, 100, None, None, ""),      # 48000-16500 = 31500 -> 5250
    ("A06", "ARRIVAL", 60000, None, None, None, 150000, 100000, 0, None, None, ""),
    ("C01", "CHARGE_LB", None, None, None, None, None, None, None, 50000, 30000, ""),      # 30000*3600/50000 = 2160 s
    ("C02", "CHARGE_LB", None, None, None, None, None, None, None, 11000, 1, ""),          # 0.327 s -> 0
    ("C03", "CHARGE_LB", None, None, None, None, None, None, None, 0, 1000, ""),
    ("C04", "CHARGE_LB", None, None, None, None, None, None, None, 7400, None, ""),
    ("C05", "CHARGE_LB", None, None, None, None, None, None, None, None, 1000, ""),
    ("C06", "CHARGE_LB", None, None, None, None, None, None, None, 7400, -1, ""),
    ("K01", "CALIBRATE", None, None, None, None, None, None, None, None, None, "3000:20000;4500:30000"),   # 7500/50 km = 150 Wh/km
    ("K02", "CALIBRATE", None, None, None, None, None, None, None, None, None, "1000:10000;9000:40000"),   # sum-first 200 Wh/km, not 162.5
    ("K03", "CALIBRATE", None, None, None, None, None, None, None, None, None, "1000:19999"),             # below 20 km
    ("K04", "CALIBRATE", None, None, None, None, None, None, None, None, None, "1001:20000"),              # 50.05 -> 50050 exactly
    ("K05", "CALIBRATE", None, None, None, None, None, None, None, None, None, "3333:20001"),              # ceil keeps it conservative
    ("K06", "CALIBRATE", None, None, None, None, None, None, None, None, None, "0:20000"),
    ("K07", "CALIBRATE", None, None, None, None, None, None, None, None, None, ""),
]

def build():
    out = []
    for r in ROWS:
        row = {k: ("" if v is None else str(v)) for k, v in zip(FIELDS[:-1], r)}
        row["expected"] = expect(row)
        out.append(row)
    return out

def main():
    path = Path(__file__).with_name("trip-energy-v1.csv")
    rows = build()
    if "--check" in sys.argv:
        with path.open(newline="") as f:
            current = list(csv.DictReader(f))
        if len(current) != len(rows) or any(a != b for a, b in zip(current, rows)):
            sys.exit("trip-energy-v1.csv differs from the independent oracle")
        print("ok", len(rows), "rows")
        return
    with path.open("w", newline="") as f:
        w = csv.DictWriter(f, FIELDS, lineterminator="\n")
        w.writeheader()
        w.writerows(rows)
    print("wrote", len(rows), "rows")

if __name__ == "__main__":
    main()
