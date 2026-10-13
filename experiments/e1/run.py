"""E1: scaling of orgc verify and orgc render.

Generates the synthetic models, builds orgc, and measures:

  cli     wall time of `orgc verify` and `orgc render` as separate processes —
          what an administrator waits for, process start and schema compile
          included; RUNS runs per model, the first one dropped as a warm-up;
  stages  each stage on its own (schema and parsing, lowering, every
          invariant) through TestE1Timings in orgc/internal/verify.

Writes results/models.json (what each model holds), cli.json, stages.json,
env.txt (hardware and versions) and e1.md (the tables). Run from the
repository root, in the iac-runner image: experiments/e1/run.sh.
"""

import json
import os
import platform
import shutil
import statistics
import subprocess
import sys
import time

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
E1 = os.path.join(ROOT, "experiments", "e1")
OUT = os.path.join(E1, "out")
RES = os.path.join(E1, "results")
RUNS = int(os.environ.get("E1_RUNS", "11"))
SEED = 1

# name → generator arguments; the three series share their middle points
MODELS = {
    "size-0010": ["--devices", "10"],
    "size-0050": ["--devices", "50"],
    "size-0200": ["--devices", "200"],
    "size-1000": ["--devices", "1000"],
    "nets-08": ["--devices", "200", "--extra-networks", "4"],
    "nets-16": ["--devices", "200", "--extra-networks", "12"],
    "nets-32": ["--devices", "200", "--extra-networks", "28"],
    "rules-0500": ["--devices", "1000", "--extra-rules", "500"],
    "rules-2000": ["--devices", "1000", "--extra-rules", "2000"],
    "rules-5000": ["--devices", "1000", "--extra-rules", "5000"],
}


def sh(*cmd, **kw):
    return subprocess.run(cmd, check=True, text=True, capture_output=True, **kw)


def stats(xs):
    xs = sorted(xs)
    q = statistics.quantiles(xs, n=4) if len(xs) > 1 else [xs[0]] * 3
    return {"median_s": statistics.median(xs), "q1_s": q[0], "q3_s": q[2], "min_s": xs[0], "max_s": xs[-1]}


def timed(cmd):
    start = time.perf_counter()
    r = subprocess.run(cmd, capture_output=True, text=True)
    elapsed = time.perf_counter() - start
    if r.returncode != 0:
        sys.exit(f"{' '.join(cmd)}: код {r.returncode}\n{r.stdout}{r.stderr}")
    return elapsed


def main():
    shutil.rmtree(OUT, ignore_errors=True)
    models = os.path.join(OUT, "models")
    os.makedirs(models)
    os.makedirs(RES, exist_ok=True)

    info = {}
    for name, args in MODELS.items():
        path = os.path.join(models, name + ".yml")
        text = sh(sys.executable, os.path.join(E1, "generate.py"), *args, "--seed", str(SEED)).stdout
        with open(path, "w", encoding="utf-8", newline="\n") as f:
            f.write(text)
        info[name] = {"args": args, "lines": text.count("\n")}
    print(f"моделі: {len(MODELS)}")

    binary = os.path.join(OUT, "orgc")
    sh("go", "build", "-o", binary, "./cmd/orgc", cwd=os.path.join(ROOT, "orgc"))

    cli = {}
    for name in MODELS:
        path = os.path.join(models, name + ".yml")
        verify, render = [], []
        for i in range(RUNS):
            v = timed([binary, "verify", path])
            dest = os.path.join(OUT, "render", name)
            shutil.rmtree(dest, ignore_errors=True)
            r = timed([binary, "render", "-o", dest, path])
            if i:
                verify.append(v)
                render.append(r)
        files = sum(len(fs) for _, _, fs in os.walk(os.path.join(OUT, "render", name)))
        lines = 0
        for d, _, fs in os.walk(os.path.join(OUT, "render", name)):
            for f in fs:
                with open(os.path.join(d, f), encoding="utf-8") as fh:
                    lines += sum(1 for _ in fh)
        cli[name] = {"verify": stats(verify), "render": stats(render), "files": files, "output_lines": lines}
        print(f"{name}: verify {cli[name]['verify']['median_s']:.3f} с, render {cli[name]['render']['median_s']:.3f} с")

    stages_out = os.path.join(RES, "stages.json")
    env = dict(os.environ, ORGC_E1_MODELS=models, ORGC_E1_OUT=stages_out, ORGC_E1_RUNS=os.environ.get("E1_STAGE_RUNS", "7"))
    subprocess.run(["go", "test", "-run", "TestE1Timings", "-count=1", "-timeout", "2h",
                    "-benchtime", os.environ.get("E1_BENCHTIME", "300ms"), "./internal/verify"],
                   check=True, cwd=os.path.join(ROOT, "orgc"), env=env)
    with open(stages_out, encoding="utf-8") as f:
        stages = json.load(f)
    for name, s in stages["models"].items():
        info[name.removesuffix(".yml")].update({k: s[k] for k in ("devices", "networks", "rules", "sites")})

    for name, data in (("models.json", info), ("cli.json", {"runs": RUNS - 1, "models": cli})):
        with open(os.path.join(RES, name), "w", encoding="utf-8", newline="\n") as f:
            json.dump(data, f, ensure_ascii=False, indent=2, sort_keys=True)
            f.write("\n")

    def first(cmd):
        try:
            return sh(*cmd).stdout.strip()
        except (OSError, subprocess.CalledProcessError):
            return "?"

    cpu = next((l.split(":", 1)[1].strip() for l in open("/proc/cpuinfo") if l.startswith("model name")), platform.processor())
    mem = next((l.split()[1] for l in open("/proc/meminfo") if l.startswith("MemTotal")), "?")
    env_txt = "\n".join([
        f"cpu: {cpu}", f"cpus: {os.cpu_count()}", f"memory_kib: {mem}", f"kernel: {platform.release()}",
        f"go: {first(['go', 'version'])}", f"python: {platform.python_version()}",
        f"image: {os.environ.get('IAC_RUNNER_IMAGE', '?')}", f"host: {os.environ.get('E1_HOST', '?')}",
        f"commit: {os.environ.get('E1_COMMIT', '?')}", f"runs: cli {RUNS - 1}, stages {stages['runs']}",
    ])
    with open(os.path.join(RES, "env.txt"), "w", encoding="utf-8", newline="\n") as f:
        f.write(env_txt + "\n")

    with open(os.path.join(RES, "e1.md"), "w", encoding="utf-8", newline="\n") as f:
        f.write(table(info, cli, stages))
    print(open(os.path.join(RES, "e1.md"), encoding="utf-8").read())


def ms(x):
    return f"{x * 1000:.0f}" if x >= 0.1 else f"{x * 1000:.1f}"


def table(info, cli, stages):
    rows = ["| Модель | Пристроїв | Сайтів | Мереж | Правил | Рядків моделі | verify, мс | render, мс | Файлів | Рядків виходу |",
            "|---|---|---|---|---|---|---|---|---|---|"]
    for name in MODELS:
        i, c = info[name], cli[name]
        v, r = c["verify"], c["render"]
        rows.append(f"| {name} | {i['devices']} | {i['sites']} | {i['networks']} | {i['rules']} | {i['lines']} | "
                    f"{ms(v['median_s'])} ({ms(v['q1_s'])}–{ms(v['q3_s'])}) | {ms(r['median_s'])} ({ms(r['q1_s'])}–{ms(r['q3_s'])}) | "
                    f"{c['files']} | {c['output_lines']} |")
    codes = sorted(next(iter(stages["models"].values()))["checks"])
    rows += ["", "Етапи verify, медіана, мкс:", "",
             "| Модель | схема і розбір | lower | " + " | ".join(codes) + " | усі перевірки |",
             "|---|---|---|" + "---|" * len(codes) + "---|"]
    for name in MODELS:
        s = stages["models"][name + ".yml"]
        cells = [f"{s['load']['median_us']:.0f}", f"{s['lower']['median_us']:.0f}"]
        cells += [f"{s['checks'][c]['median_us']:.1f}" for c in codes]
        cells.append(f"{s['verify_all']['median_us']:.0f}")
        rows.append(f"| {name} | " + " | ".join(cells) + " |")
    return "\n".join(rows) + "\n"


if __name__ == "__main__":
    main()
