"""Numbers about orgc that depend on the code, collected in one pass.

  code          lines of Go per package: code without blank lines and
                comments, tests without blank lines
  coverage      statement coverage per package (go test -cover) and in total
  tests         top-level test functions, and test runs with subtests
                (go test -json)
  corpus        the models of orgc/testdata by category
  render        files and lines orgc render produces for the examples
  diagnostics   what orgc verify prints for the models that documentation
                quotes, verbatim, with the exit code

Writes <out>/metrics.json, <out>/metrics.md and <out>/diagnostics/*.txt.
Everything is a function of the commit: a second run gives the same files.
Run from the repository root, in the iac-runner image: scripts/metrics.sh.

    metrics.py <out>
"""

import json
import os
import re
import shutil
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ORGC = os.path.join(ROOT, "orgc")

EXAMPLES = ["lab", "s-profile", "m-profile"]

# model → what verify prints for it; the path is given relative to the
# model's own directory, so the output names the file the way it is quoted
DIAGNOSTICS = [
    "examples/organization.s-profile.yml",
    "examples/organization.broken.yml",
    "examples/organization.invariants-broken.yml",
    "orgc/testdata/broken/e006-ssh-only.yml",
    "orgc/testdata/broken/e006-mgmt-not-routed.yml",
    "orgc/testdata/broken/e006-mgmt-pruned.yml",
    "orgc/testdata/broken/w006-standby-no-mgmt.yml",
    "experiments/e4/incident/lab-e006.yml",
]


def run(*cmd, cwd=ORGC, check=True):
    r = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, encoding="utf-8")
    if check and r.returncode != 0:
        sys.exit(f"{' '.join(cmd)}: код {r.returncode}\n{r.stdout}{r.stderr}")
    return r


def go_lines(path, test):
    """Lines of a Go file: tests without blank lines, code also without
    comments (line comments and /* */ blocks)."""
    n, block = 0, False
    with open(path, encoding="utf-8") as f:
        for line in f:
            s = line.strip()
            if not s:
                continue
            if test:
                n += 1
                continue
            if block:
                if "*/" in s:
                    block = False
                continue
            if s.startswith("//"):
                continue
            if s.startswith("/*"):
                block = "*/" not in s
                continue
            n += 1
    return n


def code():
    out = {}
    for d, _, files in os.walk(ORGC):
        gofiles = sorted(f for f in files if f.endswith(".go"))
        if not gofiles:
            continue
        pkg = os.path.relpath(d, ORGC).replace(os.sep, "/")
        c = sum(go_lines(os.path.join(d, f), False) for f in gofiles if not f.endswith("_test.go"))
        t = sum(go_lines(os.path.join(d, f), True) for f in gofiles if f.endswith("_test.go"))
        out[pkg] = {"code": c, "tests": t}
    return dict(sorted(out.items()))


def coverage(tmp):
    profile = os.path.join(tmp, "cover.out")
    r = run("go", "test", "-count=1", "-cover", "-coverprofile=" + profile, "./...")
    per = {}
    for line in r.stdout.splitlines():
        m = re.match(r"(?:ok|---)\s+orgc/(\S+)\s.*coverage: ([\d.]+)% of statements", line)
        if m:
            per[m.group(1)] = float(m.group(2))
    total = run("go", "tool", "cover", "-func=" + profile).stdout.strip().splitlines()[-1]
    return per, float(total.split()[-1].rstrip("%"))


def tests():
    funcs = 0
    for d, _, files in os.walk(ORGC):
        for f in files:
            if f.endswith("_test.go"):
                with open(os.path.join(d, f), encoding="utf-8") as fh:
                    funcs += sum(1 for l in fh if re.match(r"func Test\w*\(t \*testing\.T\)", l))
    r = run("go", "test", "-count=1", "-json", "./...")
    runs = passed = skipped = 0
    for line in r.stdout.splitlines():
        e = json.loads(line)
        if e.get("Test") is None:
            continue
        a = e.get("Action")
        runs += a == "run"
        passed += a == "pass"
        skipped += a == "skip"
    return {"functions": funcs, "runs": runs, "passed": passed, "skipped": skipped}


def corpus():
    td = os.path.join(ORGC, "testdata")
    broken = sorted(os.listdir(os.path.join(td, "broken")))
    structural = [f for f in broken if f.startswith("s") or f == "aggregate.yml"]
    invariant = [f for f in broken if f[0] in "ew" or f == "invariants-aggregate.yml"]
    golden = sum(len(fs) for _, _, fs in os.walk(os.path.join(td, "golden")))
    return {"valid": len(os.listdir(os.path.join(td, "valid"))), "broken_structural": len(structural),
            "broken_invariants": len(invariant), "golden_files": golden}


def render(tmp, binary):
    out = {}
    for name in EXAMPLES:
        dest = os.path.join(tmp, "render", name)
        run(binary, "render", "-o", dest, os.path.join(ROOT, "examples", f"organization.{name}.yml"))
        files = {}
        for d, _, fs in os.walk(dest):
            for f in fs:
                p = os.path.relpath(os.path.join(d, f), dest).replace(os.sep, "/")
                if p.startswith("."):
                    continue  # the output-directory marker
                with open(os.path.join(d, f), encoding="utf-8") as fh:
                    files[p] = fh.read().count("\n")
        out[name] = {"files": len(files), "lines": sum(files.values()), "by_file": dict(sorted(files.items()))}
    return out


def diagnostics(out, binary):
    os.makedirs(os.path.join(out, "diagnostics"), exist_ok=True)
    res = {}
    for model in DIAGNOSTICS:
        path = os.path.join(ROOT, model)
        r = run(binary, "verify", os.path.basename(path), cwd=os.path.dirname(path), check=False)
        text = r.stdout + r.stderr
        name = os.path.basename(model).removesuffix(".yml")
        with open(os.path.join(out, "diagnostics", name + ".txt"), "w", encoding="utf-8", newline="\n") as f:
            f.write(f"$ orgc verify {os.path.basename(path)}\n{text}(код виходу {r.returncode})\n")
        res[model] = {"exit": r.returncode, "lines": text.count("\n"),
                      "summary": next((l for l in reversed(text.splitlines()) if l.strip()), "")}
    return res


def main(out):
    tmp = os.path.join(out, "tmp")
    shutil.rmtree(out, ignore_errors=True)
    os.makedirs(tmp)
    binary = os.path.join(tmp, "orgc")
    run("go", "build", "-o", binary, "./cmd/orgc")

    m = {"go": run("go", "version").stdout.strip(),
         "commit": run("git", "rev-parse", "--short", "HEAD", cwd=ROOT, check=False).stdout.strip() or os.environ.get("METRICS_COMMIT", "?")}
    m["code"] = code()
    per, total = coverage(tmp)
    for pkg, v in m["code"].items():
        v["coverage"] = per.get(pkg)
    m["code_total"] = {"code": sum(v["code"] for v in m["code"].values()), "tests": sum(v["tests"] for v in m["code"].values()),
                       "coverage": total}
    m["tests"] = tests()
    m["corpus"] = corpus()
    m["render"] = render(tmp, binary)
    m["diagnostics"] = diagnostics(out, binary)
    shutil.rmtree(tmp)

    with open(os.path.join(out, "metrics.json"), "w", encoding="utf-8", newline="\n") as f:
        json.dump(m, f, ensure_ascii=False, indent=2)
        f.write("\n")
    with open(os.path.join(out, "metrics.md"), "w", encoding="utf-8", newline="\n") as f:
        f.write(report(m))
    print(open(os.path.join(out, "metrics.md"), encoding="utf-8").read())


def pct(v):
    return "—" if v is None else f"{v:.1f} %".replace(".", ",")


def report(m):
    rows = [f"# Числа orgc на коміті {m['commit']}", "", f"`{m['go']}`", "",
            "## Обсяг коду і покриття", "",
            "| Пакет | Код, рядків | Тести, рядків | Покриття |", "|---|---|---|---|"]
    for pkg, v in m["code"].items():
        rows.append(f"| `{pkg}` | {v['code']} | {v['tests'] or '—'} | {pct(v['coverage'])} |")
    t = m["code_total"]
    rows.append(f"| **Разом** | {t['code']} | {t['tests']} | {pct(t['coverage'])} (загальне за операторами) |")
    s = m["tests"]
    rows += ["", f"Тестових функцій: {s['functions']}; прогонів з підтестами: {s['runs']} "
                 f"(успішних {s['passed']}, пропущених {s['skipped']}).", "",
             "## Корпус testdata", "", "| Каталог | Файлів |", "|---|---|"]
    c = m["corpus"]
    rows += [f"| `testdata/valid` | {c['valid']} |", f"| `testdata/broken`, структурні | {c['broken_structural']} |",
             f"| `testdata/broken`, інваріанти | {c['broken_invariants']} |", f"| `testdata/golden` | {c['golden_files']} |",
             "", "## Вихід orgc render для прикладів", ""]
    for name, r in m["render"].items():
        rows += [f"### {name}: {r['files']} файлів, {r['lines']} рядків", "", "| Файл | Рядків |", "|---|---|"]
        rows += [f"| `{p}` | {n} |" for p, n in r["by_file"].items()]
        rows.append("")
    rows += ["## Діагностики (повний вивід — diagnostics/)", "", "| Модель | Код виходу | Рядків | Підсумок |", "|---|---|---|---|"]
    for model, d in m["diagnostics"].items():
        rows.append(f"| `{model}` | {d['exit']} | {d['lines']} | {d['summary']} |")
    return "\n".join(rows) + "\n"


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("використання: metrics.py <каталог виходу>")
    main(os.path.abspath(sys.argv[1]))
