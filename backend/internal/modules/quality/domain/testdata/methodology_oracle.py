#!/usr/bin/env python3
"""Independent oracle for the FQ-12 sample-judgment methodology.

Mirrors the official ANP PMQC open-data guidance (Oct 2026) directly from its
text: assay is the file unit, sample (full CNPJ + collection clock + IdNumeric + Produto) is the judgment
unit, Aspecto has fuel/epoch exceptions, replicas collapse, and undecidable
cases stay UNRESOLVED. It never converts qualitative results to numbers.
"""
import csv
import collections
import sys
import unicodedata


def fold(text):
    return unicodedata.normalize("NFKD", text).encode("ascii", "ignore").decode().lower().strip()


def fuel_family(produto):
    p = produto.upper()
    if p.startswith("GASOLINA"):
        return "gasoline"
    if p.startswith("ETANOL"):
        return "ethanol"
    if "DIESEL" in p:
        return "diesel_s10" if "S10" in p else "diesel_s500"
    return "other"


def defect(result):
    f = fold(result)
    if "heterog" in f:
        return "heterogeneous"
    if "material particulado" in f:
        return "particles"
    if f.startswith("turvo"):
        return "turbidity"
    return "unknown"


def collapse(rows):
    """rows: list of (ensaio, resultado, conforme). None means replica conflict."""
    defects = {r[1] for r in rows if r[0] == "Aspecto"}
    if len(defects) > 1:
        return None
    by = collections.OrderedDict()
    for ensaio, _resultado, conforme in rows:
        by.setdefault(ensaio, []).append(conforme)
    out = {}
    for ensaio, values in by.items():
        statuses = set(values)
        if len(statuses) > 1:
            return None
        out[ensaio] = statuses.pop()
    return out


def outcome(assays, names):
    """(verdict, cleared): any Não fails; every required present and Sim clears."""
    present = cleared = False
    for name in names:
        if name not in assays:
            continue
        present = True
        if assays[name] == "Não":
            return "NONCONFORMING", False
        cleared = True
    return "CONFORMING", present and cleared


def judge(rows, produto, datacoleta):
    assays = collapse([(r[0], r[1], r[2]) for r in rows])
    if assays is None:
        return "UNRESOLVED", "replica_conflict"
    family = fuel_family(produto)
    if any(c not in ("Sim", "Não") for c in assays.values()):
        return "UNRESOLVED", "unknown_assay_status"
    nao = sorted(n for n, c in assays.items() if c == "Não")
    if [n for n in nao if n != "Aspecto"]:
        return "NONCONFORMING", ""
    if "Aspecto" not in nao:
        return "CONFORMING", ""
    if family == "gasoline" or family == "other":
        return "NONCONFORMING", ""
    if family == "ethanol":
        comps = [n for n in ("Resíduo por Evaporação", "Material Não Volátil", "Teor de Material Não Volátil") if n in assays]
        if any(assays[n] == "Não" for n in comps):
            return "NONCONFORMING", ""
        if any(assays[n] == "Sim" for n in comps):
            return "CONFORMING", "ethanol_aspecto_complementary_conforming"
        return "CONFORMING", "aspecto_vice_unconfirmed"
    # diesel
    aspect_result = next((r[1] for r in rows if r[0] == "Aspecto"), "")
    grade_set = ["Teor de Água", "Teor de Água e Sedimentos"] if family == "diesel_s500" else ["Teor de Água", "Contaminação total"]
    legacy_verdict, legacy_cleared = outcome(assays, grade_set)
    kind = defect(aspect_result)
    if kind == "heterogeneous":
        current_verdict, current_cleared = "NONCONFORMING", False
    elif kind == "particles":
        current_verdict, current_cleared = outcome(assays, ["Contaminação total"])
    elif kind == "turbidity":
        current_verdict, current_cleared = outcome(assays, ["Teor de Água", "Contaminação total"])
    else:
        current_verdict, current_cleared = "UNRESOLVED", False
    year = datacoleta[:4]
    if year >= "2026":
        verdict, cleared = current_verdict, current_cleared
        if verdict == "UNRESOLVED":
            return "UNRESOLVED", "diesel_defect_unclassified"
    elif year <= "2023":
        verdict, cleared = legacy_verdict, legacy_cleared
    else:
        if current_verdict == "UNRESOLVED":
            return "UNRESOLVED", "diesel_defect_unclassified"
        if current_verdict != legacy_verdict:
            return "UNRESOLVED", "diesel_rule_epoch_unverified"
        verdict, cleared = current_verdict, current_cleared or legacy_cleared
    if verdict != "CONFORMING":
        return verdict, ""
    if cleared:
        return "CONFORMING", "diesel_aspecto_complementary_conforming"
    return "CONFORMING", "aspecto_vice_unconfirmed"


def analyze(path):
    samples = collections.defaultdict(list)
    with open(path, encoding="utf-8-sig", newline="") as f:
        for row in csv.DictReader(f, delimiter=";"):
            samples[(row["IdNumeric"], row["Produto"], row["CnpjPosto"], row["DataColeta"])].append((row["Ensaio"], row["Resultado"], row["Conforme"], row["DataColeta"]))
    stats = collections.defaultdict(collections.Counter)
    reasons = collections.Counter()
    aspect = collections.Counter()
    replicas = 0
    for (idnumeric, produto, _cnpj, _clock), rows in samples.items():
        verdict, reason = judge([(r[0], r[1], r[2]) for r in rows], produto, rows[0][3])
        family = fuel_family(produto)
        stats[family][verdict] += 1
        reasons[reason] += 1
        if any(r[0] == "Aspecto" and r[2] == "Não" for r in rows):
            aspect[(family, verdict, reason)] += 1
        counts = collections.Counter(r[0] for r in rows)
        replicas += sum(v - 1 for v in counts.values() if v > 1)
    return stats, reasons, aspect, len(samples), replicas


if __name__ == "__main__":
    for path in sys.argv[1:]:
        stats, reasons, aspect, total, replicas = analyze(path)
        print(f"{path}: samples={total} replica_rows={replicas}")
        for family in sorted(stats):
            print(f"  {family}: {dict(stats[family])}")
        print(f"  reasons: {dict(sorted(reasons.items()))}")
        for case, n in sorted(aspect.items()):
            print(f"    aspecto_case {case}: {n}")
