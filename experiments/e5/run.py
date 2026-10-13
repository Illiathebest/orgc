"""E5: locality of the diff — how much of the output one change of the model
touches.

For each base model and each single change, the base and the changed model
are rendered (under the same file name, so the generated headers agree) and
the two output trees are compared: files changed, added and removed, lines
added plus removed, and their share of the whole output. A change is local
when it touches only the devices and model-wide artefacts it concerns.

Writes results/e5.json (every change: the files it touched and the counts)
and results/e5.md. Run from the repository root, in the iac-runner image:
experiments/e5/run.sh.
"""

import copy
import difflib
import json
import os
import shutil
import subprocess
import sys

import yaml

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
E5 = os.path.join(ROOT, "experiments", "e5")
OUT = os.path.join(E5, "out")
RES = os.path.join(E5, "results")

BASES = {"m-profile": "examples/organization.m-profile.yml", "synthetic-200": 200}


def first(site, role):
    return next(d for d in site["devices"] if d["role"] == role)


def net(site, role):
    return next(n for n in site["networks"] if n["role"] == role)


def free_eos_port(site, dev):
    used = set()
    for l in site.get("links", []):
        for end in (l["a"], l["b"]):
            d, p = end.split(":")
            if d == dev:
                used.add(p)
    for s in site.get("servers", []):
        if s.get("port", "").startswith(dev + ":"):
            used.add(s["port"].split(":")[1])
    for d in site["devices"]:
        if d["name"] == dev:
            for ap in d.get("access_ports", []):
                used |= set(ap["ports"])
    return next(f"Ethernet{i}" for i in range(1, 49) if f"Ethernet{i}" not in used)


# Every change: (name, what it does in words, function on the parsed model).
# They act on the first site, the control node's, unless they say otherwise.

def add_rule(m):
    s = m["sites"][0]
    m["policies"]["inter_vlan"].append({"from": net(s, "users")["name"], "to": net(s, "servers")["name"], "ports": [8080], "action": "allow"})


def change_rule_ports(m):
    s = m["sites"][0]
    r = next(r for r in m["policies"]["inter_vlan"] if r["from"] == net(s, "users")["name"] and r["to"] == net(s, "servers")["name"])
    r["ports"] = sorted(set(r["ports"]) | {8443})


def add_network(m):
    s = m["sites"][0]
    used = {n["vlan"] for n in s["networks"]}
    vlan = next(v for v in range(50, 4095) if v not in used)
    s["networks"].append({"vlan": vlan, "name": "voice", "role": "users", "cidr": "10.252.0.0/24", "gateway": "10.252.0.1",
                          "dhcp_pool": {"from": "10.252.0.100", "to": "10.252.0.200"}})


def add_device(m):
    s = m["sites"][0]
    parent = first(s, "distribution_switch")
    mg = net(s, "mgmt")
    taken = {d["mgmt_ip"] for d in s["devices"]}
    ip = next(str(h) for h in __import__("ipaddress").ip_network(mg["cidr"]).hosts() if str(h) not in taken and str(h) != mg.get("gateway"))
    s["devices"].append({"name": "acc-new", "type": "arista_eos", "os_version": "4.36.2F", "role": "access_switch", "mgmt_ip": ip})
    s["links"].append({"a": f"{parent['name']}:{free_eos_port(s, parent['name'])}", "b": "acc-new:Ethernet1",
                       "vlans": [mg["name"], net(s, "users")["name"]]})


def remove_standby(m):
    s = m["sites"][0]
    s["links"].remove(next(l for l in s["links"] if l.get("standby")))


class NotApplicable(Exception):
    """The base has nowhere to make this change."""


def vlan_on_trunk(m):
    """Allow on an active trunk a network of the site it does not carry."""
    s = m["sites"][0]
    for l in s["links"]:
        if "vlans" in l and not l.get("standby"):
            missing = [n["name"] for n in s["networks"] if n["name"] not in l["vlans"]]
            if missing:
                l["vlans"].append(missing[0])
                return
    raise NotApplicable("кожен trunk уже несе всі мережі сайту")


def below(site, link):
    """Devices under an active link: on the far side from the gateway."""
    gw = next(d["name"] for d in site["devices"] if d["role"] in ("edge_router", "firewall"))
    adj = {}
    for l in site["links"]:
        if l is not link and not l.get("standby"):
            a, b = l["a"].split(":")[0], l["b"].split(":")[0]
            adj.setdefault(a, []).append(b)
            adj.setdefault(b, []).append(a)
    seen, todo = {gw}, [gw]
    while todo:
        for w in adj.get(todo.pop(), []):
            if w not in seen:
                seen.add(w)
                todo.append(w)
    return {d["name"] for d in site["devices"]} - seen


def needed(site, devices):
    """Networks the devices need delivered: mgmt, access ports, servers."""
    import ipaddress
    out = set()
    for d in site["devices"]:
        if d["name"] in devices:
            out |= {n["name"] for n in site["networks"] if ipaddress.ip_address(d["mgmt_ip"]) in ipaddress.ip_network(n["cidr"])}
            out |= {ap["network"] for ap in d.get("access_ports", [])}
    for sv in site.get("servers", []):
        if sv.get("port", "").split(":")[0] in devices:
            out |= {n["name"] for n in site["networks"] if ipaddress.ip_address(sv["ip"]) in ipaddress.ip_network(n["cidr"])}
    return out


def vlan_off_trunk(m):
    """Prune from an active trunk a network nothing below it needs."""
    s = m["sites"][0]
    for l in s["links"]:
        if l.get("standby"):
            continue
        carried = l.get("vlans", [n["name"] for n in s["networks"]])
        spare = [n for n in carried if n not in needed(s, below(s, l))]
        if spare:
            l["vlans"] = [n for n in carried if n != spare[0]]
            return
    raise SystemExit("немає trunk з мережею, яка нижче не потрібна")


def change_mgmt_ip(m):
    s = m["sites"][0]
    d = first(s, "access_switch")
    taken = {x["mgmt_ip"] for x in s["devices"]}
    mg = net(s, "mgmt")
    d["mgmt_ip"] = next(str(h) for h in __import__("ipaddress").ip_network(mg["cidr"]).hosts()
                        if str(h) not in taken and str(h) != mg.get("gateway") and int(str(h).split(".")[-1]) > 100)


def add_server(m):
    s = m["sites"][0]
    acc = first(s, "access_switch")
    srv = net(s, "servers")
    taken = {x["ip"] for x in s.get("servers", [])} | {m["control_node"]["ip"]}
    ip = next(str(h) for h in __import__("ipaddress").ip_network(srv["cidr"]).hosts()
              if str(h) not in taken and str(h) != srv.get("gateway") and int(str(h).split(".")[-1]) > 50)
    s["servers"].append({"name": "app-new", "role": ["app"], "ip": ip, "port": f"{acc['name']}:{free_eos_port(s, acc['name'])}"})


def other_site_rule(m):
    """A rule inside the last site: nothing of the first site may change."""
    s = m["sites"][-1]
    m["policies"]["inter_vlan"].append({"from": net(s, "users")["name"], "to": net(s, "servers")["name"], "ports": [8080], "action": "allow"})


CHANGES = [
    ("rule-add", "нове правило users → servers у сайті вузла керування", add_rule),
    ("rule-ports", "до наявного правила додано порт", change_rule_ports),
    ("rule-other-site", "нове правило в останньому сайті", other_site_rule),
    ("network-add", "нова мережа з DHCP у сайті", add_network),
    ("device-add", "новий access-комутатор під distribution", add_device),
    ("standby-remove", "прибрано резервне зʼєднання access-комутатора", remove_standby),
    ("vlan-on-trunk", "на trunk дозволено ще один VLAN сайту", vlan_on_trunk),
    ("vlan-off-trunk", "з trunk прибрано VLAN, не потрібний нижче", vlan_off_trunk),
    ("mgmt-ip", "змінено mgmt_ip access-комутатора", change_mgmt_ip),
    ("server-add", "новий сервер на access-порту", add_server),
]


def tree(root):
    out = {}
    for d, _, files in os.walk(root):
        for f in files:
            full = os.path.join(d, f)
            rel = os.path.relpath(full, root).replace(os.sep, "/")
            with open(full, encoding="utf-8") as fh:
                out[rel] = fh.read().splitlines()
    return out


def render(binary, model, where):
    os.makedirs(where, exist_ok=True)
    path = os.path.join(where, "organization.yml")
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        yaml.safe_dump(model, f, allow_unicode=True, sort_keys=False, width=200)
    dest = os.path.join(where, "render")
    r = subprocess.run([binary, "render", "-o", dest, path], capture_output=True, text=True, encoding="utf-8")
    if r.returncode != 0:
        sys.exit(f"orgc render {where}: код {r.returncode}\n{r.stdout}{r.stderr}")
    return tree(dest)


def main():
    shutil.rmtree(OUT, ignore_errors=True)
    os.makedirs(OUT)
    os.makedirs(RES, exist_ok=True)
    binary = os.path.join(OUT, "orgc")
    subprocess.run(["go", "build", "-o", binary, "./cmd/orgc"], check=True, cwd=os.path.join(ROOT, "orgc"))

    results = {}
    for bname, src in BASES.items():
        if isinstance(src, int):
            text = subprocess.run([sys.executable, os.path.join(ROOT, "experiments", "e1", "generate.py"), "--devices", str(src)],
                                  check=True, capture_output=True, text=True, encoding="utf-8").stdout
        else:
            with open(os.path.join(ROOT, src), encoding="utf-8") as f:
                text = f.read()
        base_model = yaml.safe_load(text)
        base = render(binary, base_model, os.path.join(OUT, bname, "base"))
        size = {"files": len([p for p in base if not p.startswith(".")]), "lines": sum(len(v) for p, v in base.items() if not p.startswith("."))}
        results[bname] = {"output": size, "changes": {}}
        for cname, desc, f in CHANGES:
            m = copy.deepcopy(base_model)
            try:
                f(m)
            except NotApplicable as e:
                results[bname]["changes"][cname] = {"what": desc, "not_applicable": str(e)}
                print(f"{bname} / {cname}: не застосовна — {e}")
                continue
            new = render(binary, m, os.path.join(OUT, bname, cname))
            changed, added, removed, lines = [], [], [], 0
            for p in sorted(set(base) | set(new)):
                if p.startswith("."):
                    continue  # the output-directory marker
                a, b = base.get(p), new.get(p)
                if a == b:
                    continue
                if a is None:
                    added.append(p)
                elif b is None:
                    removed.append(p)
                else:
                    changed.append(p)
                lines += sum(1 for l in difflib.unified_diff(a or [], b or [], lineterm="", n=0)
                             if l[:1] in "+-" and not l.startswith(("+++", "---")))
            results[bname]["changes"][cname] = {"what": desc, "changed": changed, "added": added, "removed": removed,
                                                "files": len(changed) + len(added) + len(removed), "lines": lines}
            print(f"{bname} / {cname}: файлів {len(changed) + len(added) + len(removed)}, рядків {lines}")

    with open(os.path.join(RES, "e5.json"), "w", encoding="utf-8", newline="\n") as f:
        json.dump(results, f, ensure_ascii=False, indent=2)
        f.write("\n")
    with open(os.path.join(RES, "e5.md"), "w", encoding="utf-8", newline="\n") as f:
        f.write(table(results))
    print(open(os.path.join(RES, "e5.md"), encoding="utf-8").read())


def table(results):
    rows = []
    for bname, r in results.items():
        size = r["output"]
        rows += [f"### {bname}: вихід — {size['files']} файлів, {size['lines']} рядків", "",
                 "| Зміна | Що змінено | Файлів (змінено + додано − видалено) | Частка файлів | Рядків +/− | Частка рядків | Які файли |",
                 "|---|---|---|---|---|---|---|"]
        for cname, c in r["changes"].items():
            if "not_applicable" in c:
                rows.append(f"| {cname} | {c['what']} | — | — | — | — | не застосовна: {c['not_applicable']} |")
                continue
            files = ", ".join([f"`{p}`" for p in c["changed"]] + [f"+`{p}`" for p in c["added"]] + [f"−`{p}`" for p in c["removed"]])
            rows.append(f"| {cname} | {c['what']} | {c['files']} ({len(c['changed'])} + {len(c['added'])} − {len(c['removed'])}) | "
                        f"{100 * c['files'] / size['files']:.1f} % | {c['lines']} | {100 * c['lines'] / size['lines']:.2f} % | {files} |")
        rows.append("")
    return "\n".join(rows)


if __name__ == "__main__":
    main()
