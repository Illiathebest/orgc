"""E3: abstraction ratio — lines of the model against lines orgc generates.

For every example, every model of E4 and the synthetic models of E1, the
model is rendered and lines are counted with blanks and comments left out:
`#` in YAML and RouterOS scripts, `!` in EOS configurations, the YAML
document marker `---`. The output is counted by artifact type; the ratio is
output lines per model line.

How many lines a model takes depends on how it is written: the examples
put a device over several lines, the synthetic models of E1 put it on one.
So the model is also counted in a canonical form — parsed and written back
in YAML block style — and a second ratio is given against that.

Writes results/e3.json and results/e3.md. Run from the repository root, in
the iac-runner image: experiments/e3/run.sh.
"""

import json
import os
import shutil
import subprocess
import sys

import yaml

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
E3 = os.path.join(ROOT, "experiments", "e3")
OUT = os.path.join(E3, "out")
RES = os.path.join(E3, "results")

MODELS = {
    "lab": "examples/organization.lab.yml",
    "s-profile": "examples/organization.s-profile.yml",
    "m-profile": "examples/organization.m-profile.yml",
    "e4-minimal": "experiments/e4/models/minimal.yml",
    "e4-eos-gateway": "experiments/e4/models/eos-gateway.yml",
    "e4-capacity": "experiments/e4/models/capacity.yml",
    "synthetic-0010": 10,
    "synthetic-0050": 50,
    "synthetic-0200": 200,
    "synthetic-1000": 1000,
}

# artifact type by path in the output tree (docs/adr/ADR-009, decision 2)
TYPES = [
    ("RouterOS .rsc", lambda p: p.endswith(".rsc")),
    ("EOS .cfg", lambda p: p.endswith(".cfg")),
    ("Ansible", lambda p: p.startswith("ansible/")),
    ("Prometheus", lambda p: p.startswith("prometheus/")),
    ("containerlab", lambda p: p.startswith("containerlab/")),
]


def meaningful(path, text):
    comment = "!" if path.endswith(".cfg") else "#"
    n = 0
    for line in text.splitlines():
        s = line.strip()
        if s and not s.startswith(comment) and s != "---":
            n += 1
    return n


def canonical(text):
    """Lines of the model in YAML block style, whatever its style was."""
    out = yaml.safe_dump(yaml.safe_load(text), default_flow_style=False, sort_keys=False, allow_unicode=True, width=10**6)
    return meaningful("model.yml", out)


def count_tree(root):
    by_type = {t: {"files": 0, "lines": 0, "raw": 0} for t, _ in TYPES}
    for d, _, files in os.walk(root):
        for f in files:
            full = os.path.join(d, f)
            rel = os.path.relpath(full, root).replace(os.sep, "/")
            kind = next((t for t, match in TYPES if match(rel)), None)
            if kind is None:
                continue  # the output-directory marker
            with open(full, encoding="utf-8") as fh:
                text = fh.read()
            by_type[kind]["files"] += 1
            by_type[kind]["lines"] += meaningful(rel, text)
            by_type[kind]["raw"] += text.count("\n")
    return by_type


def main():
    shutil.rmtree(OUT, ignore_errors=True)
    os.makedirs(OUT)
    os.makedirs(RES, exist_ok=True)
    binary = os.path.join(OUT, "orgc")
    subprocess.run(["go", "build", "-o", binary, "./cmd/orgc"], check=True, cwd=os.path.join(ROOT, "orgc"))

    results = {}
    for name, src in MODELS.items():
        if isinstance(src, int):
            path = os.path.join(OUT, name + ".yml")
            text = subprocess.run([sys.executable, os.path.join(ROOT, "experiments", "e1", "generate.py"), "--devices", str(src)],
                                  check=True, capture_output=True, text=True, encoding="utf-8").stdout
            with open(path, "w", encoding="utf-8", newline="\n") as f:
                f.write(text)
        else:
            path = os.path.join(ROOT, src)
        with open(path, encoding="utf-8") as f:
            text = f.read()
        dest = os.path.join(OUT, "render", name)
        r = subprocess.run([binary, "render", "-o", dest, path], capture_output=True, text=True, encoding="utf-8")
        if r.returncode != 0:
            sys.exit(f"{name}: orgc render, код {r.returncode}\n{r.stdout}{r.stderr}")
        out = count_tree(dest)
        total = sum(v["lines"] for v in out.values())
        model_lines = meaningful(path, text)
        canon = canonical(text)
        results[name] = {"model_lines": model_lines, "model_canonical": canon, "model_raw": text.count("\n"), "output": out,
                         "output_lines": total, "ratio": round(total / model_lines, 2), "ratio_canonical": round(total / canon, 2)}
        print(f"{name}: модель {model_lines} ({canon} канонічно), вихід {total}, "
              f"коефіцієнт {total / model_lines:.1f} ({total / canon:.1f})")

    with open(os.path.join(RES, "e3.json"), "w", encoding="utf-8", newline="\n") as f:
        json.dump(results, f, ensure_ascii=False, indent=2, sort_keys=True)
        f.write("\n")
    with open(os.path.join(RES, "e3.md"), "w", encoding="utf-8", newline="\n") as f:
        f.write(table(results))
    print(open(os.path.join(RES, "e3.md"), encoding="utf-8").read())


def table(results):
    types = [t for t, _ in TYPES]
    rows = ["| Модель | Рядків моделі (канонічно) | " + " | ".join(types) + " | Разом виходу | Коефіцієнт (канонічно) |",
            "|---|---|" + "---|" * len(types) + "---|---|"]
    for name, r in results.items():
        cells = []
        for t in types:
            o = r["output"][t]
            cells.append(f"{o['lines']} ({o['files']})" if o["files"] else "—")
        rows.append(f"| {name} | {r['model_lines']} ({r['model_canonical']}) | " + " | ".join(cells) +
                    f" | {r['output_lines']} | {r['ratio']:.1f} ({r['ratio_canonical']:.1f}) |")
    rows += ["", "Рядки без порожніх і коментарів; у дужках при артефактах — кількість файлів. Коефіцієнт — рядків виходу на",
             "рядок моделі; «канонічно» — проти моделі, переписаної блочним стилем YAML, тобто незалежно від стилю запису.", ""]
    rows += ["Коефіцієнт за типом артефакту (рядків цього типу на рядок моделі, як її записано):", "",
             "| Модель | " + " | ".join(types) + " |", "|---|" + "---|" * len(types)]
    for name, r in results.items():
        rows.append(f"| {name} | " + " | ".join(
            f"{r['output'][t]['lines'] / r['model_lines']:.2f}" if r["output"][t]["files"] else "—" for t in types) + " |")
    return "\n".join(rows) + "\n"


if __name__ == "__main__":
    main()
