#!/usr/bin/env python3
"""Fail-closed capacity check for one extra API pod, using live quota data."""
from decimal import Decimal, InvalidOperation
import json
import re
import sys

FACTORS = {"": Decimal(1), "n": Decimal("1e-9"), "u": Decimal("1e-6"),
           "m": Decimal(".001"), "k": Decimal(1000), "M": Decimal("1e6"),
           "G": Decimal("1e9"), "T": Decimal("1e12"), "P": Decimal("1e15"),
           "E": Decimal("1e18"),
           **{p + "i": Decimal(1024) ** i for i, p in enumerate("KMGTPE", 1)}}

def quantity(raw):
    match = re.fullmatch(r"([0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)([a-zA-Z]*)", str(raw))
    if not match or match[2] not in FACTORS:
        raise ValueError("unsupported resource quantity")
    value = Decimal(match[1]) * FACTORS[match[2]]
    if not value.is_finite() or value < 0:
        raise ValueError("invalid resource quantity")
    return value

def check(deployment, quota):
    pod = deployment["spec"]["template"]["spec"]
    if pod.get("initContainers") or pod.get("overhead"):
        raise ValueError("init or overhead resources need operator review")
    if any(c["name"] == "private-storage" for c in pod["containers"]):
        raise ValueError("separate private storage before surge")
    if any(v["name"] == "dev-private-media" for v in pod.get("volumes", [])):
        raise ValueError("remove shared media mount before surge")
    status = quota["status"]
    result = {}
    for group in ("requests", "limits"):
        for resource in ("cpu", "memory"):
            key = group + "." + resource
            extra = sum(quantity(c["resources"][group][resource]) for c in pod["containers"])
            room = quantity(status["hard"][key]) - quantity(status["used"][key])
            result[key] = {"extra": str(extra), "headroom": str(room)}
            if room < extra:
                raise ValueError("insufficient " + key + " quota for surge")
    return result

if __name__ == "__main__":
    try:
        data = json.load(sys.stdin)
        print(json.dumps(check(data["deployment"], data["quota"])))
    except (ValueError, KeyError, TypeError, InvalidOperation):
        sys.exit("surge refused: storage or complete resource headroom requires operator review")
