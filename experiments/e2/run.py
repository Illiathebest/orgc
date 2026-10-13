"""E2: how completely orgc verify catches defects.

Every operator in mutate.py is applied to every base model; up to PER_CLASS
mutants per class are drawn from the result, round-robin across the bases so
that no single model dominates a class, in an order fixed by SEED. Each
mutant goes through `orgc verify`, and the diagnostics it reports beyond
those of its base (the W002 warnings of EOS gateways, say) decide the
outcome:

  exact     only the expected code
  with      the expected code and others besides
  other     other codes, not the expected one
  missed    no new diagnostic at all
  invalid   the mutant fails the schema (S-codes): a defect of the operator,
            not of verify; reported and left out of the rates

Writes results/mutants.json (every mutant: class, base, change, new codes,
outcome) and results/e2.md (the table and every mutant that is not exact).
Run from the repository root, in the iac-runner image: experiments/e2/run.sh.
"""

import collections
import json
import os
import random
import re
import shutil
import subprocess
import sys

import yaml

import mutate

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
E2 = os.path.join(ROOT, "experiments", "e2")
OUT = os.path.join(E2, "out")
# E2_RESULTS: a subdirectory of results/, e.g. for a run against an older
# orgc given by E2_ORGC
RES = os.path.join(E2, "results", os.environ.get("E2_RESULTS", ""))
PER_CLASS = int(os.environ.get("E2_PER_CLASS", "50"))
SEED = 2

BASES = [
    "examples/organization.s-profile.yml",
    "examples/organization.m-profile.yml",
    "examples/organization.lab.yml",
    "experiments/e4/models/minimal.yml",
    "experiments/e4/models/eos-gateway.yml",
    "experiments/e4/models/capacity.yml",
    "synthetic-50",  # experiments/e1/generate.py --devices 50
    "synthetic-200",  # experiments/e1/generate.py --devices 200
]
DIAG = re.compile(r"(?:помилка|попередження) ([ESW]\d{3}): (.*)$")


def verify(binary, path):
    r = subprocess.run([binary, "verify", path], capture_output=True, text=True, encoding="utf-8")
    if r.returncode not in (0, 1):
        sys.exit(f"orgc verify {path}: код {r.returncode}\n{r.stdout}{r.stderr}")
    # (code, message): a mutant is compared with its base diagnostic by
    # diagnostic, so a warning the base already had, replaced by another
    # of the same code, still counts as new
    return [m.groups() for line in (r.stdout + r.stderr).splitlines() if (m := DIAG.search(line))]


def dump(m, path):
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        yaml.safe_dump(m, f, allow_unicode=True, sort_keys=False, width=200)


def main():
    shutil.rmtree(OUT, ignore_errors=True)
    os.makedirs(os.path.join(OUT, "mutants"))
    os.makedirs(RES, exist_ok=True)
    binary = os.environ.get("E2_ORGC")
    if not binary:
        binary = os.path.join(OUT, "orgc")
        subprocess.run(["go", "build", "-o", binary, "./cmd/orgc"], check=True, cwd=os.path.join(ROOT, "orgc"))

    bases = {}
    for b in BASES:
        if b.startswith("synthetic-"):
            text = subprocess.run([sys.executable, os.path.join(ROOT, "experiments", "e1", "generate.py"), "--devices", b.split("-")[1]],
                                  check=True, capture_output=True, text=True, encoding="utf-8").stdout
            path = os.path.join(OUT, b + ".yml")
            with open(path, "w", encoding="utf-8", newline="\n") as f:
                f.write(text)
        else:
            path = os.path.join(ROOT, b)
        with open(path, encoding="utf-8") as f:
            model = yaml.safe_load(f)
        codes = verify(binary, path)
        if any(c.startswith(("E", "S")) for c, _ in codes):
            sys.exit(f"базова модель {b} не коректна: {codes}")
        bases[b] = (model, collections.Counter(codes))

    rng = random.Random(SEED)
    results = []
    for code, op in mutate.OPERATORS.items():
        pools = []
        for b in BASES:
            ms = [(b, d, m) for d, m in op(bases[b][0])]
            rng.shuffle(ms)
            pools.append(ms)
        picked = []
        while len(picked) < PER_CLASS and any(pools):
            for p in pools:
                if p and len(picked) < PER_CLASS:
                    picked.append(p.pop())
        for n, (b, desc, m) in enumerate(picked):
            path = os.path.join(OUT, "mutants", f"{code.lower()}-{n:02d}.yml")
            dump(m, path)
            fresh = collections.Counter(verify(binary, path)) - bases[b][1]
            new = collections.Counter(code for code, _ in fresh.elements())
            got = sorted(new.elements())
            if any(c.startswith("S") for c in got):
                outcome = "invalid"
            elif not got:
                outcome = "missed"
            elif code not in new:
                outcome = "other"
            elif set(new) == {code}:
                outcome = "exact"
            else:
                outcome = "with"
            results.append({"class": code, "id": os.path.basename(path), "base": b, "change": desc,
                            "codes": got, "outcome": outcome})
        print(f"{code}: {len(picked)} мутантів, " + ", ".join(
            f"{o} {sum(1 for r in results if r['class'] == code and r['outcome'] == o)}"
            for o in ("exact", "with", "other", "missed", "invalid")))

    with open(os.path.join(RES, "mutants.json"), "w", encoding="utf-8", newline="\n") as f:
        json.dump({"seed": SEED, "per_class": PER_CLASS, "bases": BASES, "mutants": results}, f, ensure_ascii=False, indent=1)
        f.write("\n")
    with open(os.path.join(RES, "e2.md"), "w", encoding="utf-8", newline="\n") as f:
        f.write(report(results))
    print(open(os.path.join(RES, "e2.md"), encoding="utf-8").read())


def pct(a, b):
    return f"{100 * a / b:.0f} %" if b else "—"


def report(results):
    rows = ["| Клас | Мутантів | Точно | Разом з іншими | Іншим кодом | Пропущено | Невалідні | Виявлено саме цим кодом | Точність |",
            "|---|---|---|---|---|---|---|---|---|"]
    tot = collections.Counter()
    for code in mutate.OPERATORS:
        rs = [r for r in results if r["class"] == code]
        c = collections.Counter(r["outcome"] for r in rs)
        valid = len(rs) - c["invalid"]
        tot.update(c)
        tot["valid"] += valid
        rows.append(f"| {code} | {valid} | {c['exact']} | {c['with']} | {c['other']} | {c['missed']} | {c['invalid']} | "
                    f"{pct(c['exact'] + c['with'], valid)} | {pct(c['exact'], valid)} |")
    rows.append(f"| **разом** | {tot['valid']} | {tot['exact']} | {tot['with']} | {tot['other']} | {tot['missed']} | {tot['invalid']} | "
                f"{pct(tot['exact'] + tot['with'], tot['valid'])} | {pct(tot['exact'], tot['valid'])} |")
    rows += ["", "«Виявлено саме цим кодом» — очікуваний код серед нових (точно + разом з іншими); «точність» — лише очікуваний код.",
             "", "## Неточні випадки", "", "| Мутант | База | Зміна | Нові коди | Результат |", "|---|---|---|---|---|"]
    for r in results:
        if r["outcome"] != "exact":
            rows.append(f"| {r['id']} | {r['base']} | {r['change']} | {' '.join(r['codes']) or '—'} | {r['outcome']} |")
    return "\n".join(rows) + "\n"


if __name__ == "__main__":
    main()
