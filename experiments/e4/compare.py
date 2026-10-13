"""End-to-end check: expected state from the model against the devices.

Runs on the control node — in the emulation, inside the network namespace of
the server that stands in for it — so devices are read and management ports
probed along the path the model describes, through the site gateway and its
rules, not over containerlab's management network.

Every compared item is one parameter: a VLAN, an address, a port, a
management port reached. With --render, one more parameter per device says
whether it took the whole configuration orgc generated: every line of the
EOS .cfg is in the running configuration; the RouterOS script ran to its
last command (VLAN filtering on) with no script error logged. This one is
checked against orgc's output, not the model: it catches a rejected command
or an import cut short, which may leave the oracle's parameters intact. A discrepancy names the device, the parameter, and
what was expected and observed. The exit code is 1 when any parameter
differs, 0 otherwise.

Credentials come from the environment: E4_EOS_USER / E4_EOS_PASSWORD and
E4_ROS_USER / E4_ROS_PASSWORD, admin/admin by default (the emulation's
lab accounts).
"""

import argparse
import ipaddress
import json
import os
import socket
import sys
import time

import expected
import observe


def reachable(host, port, wait=0.0, timeout=3.0):
    """Try to connect, retrying until wait seconds have passed: a device
    that is still applying its startup configuration is not yet a
    discrepancy."""
    deadline = time.monotonic() + wait
    while True:
        try:
            with socket.create_connection((host, port), timeout=timeout):
                return True
        except OSError:
            if time.monotonic() >= deadline:
                return False
            time.sleep(2)


def in_model(address, networks):
    ip = ipaddress.ip_interface(address).ip
    return any(ip in ipaddress.ip_network(n, strict=False) for n in networks)


def compare_device(name, exp, obs):
    """Return (checked, discrepancies) for one device."""
    checked = 0
    diffs = []

    def differ(param, want, got):
        diffs.append({"device": name, "parameter": param, "expected": want, "observed": got})

    for vid, vname in exp["vlans"].items():
        checked += 1
        got = obs["vlans"].get(vid)
        if got != vname:
            differ(f"vlan {vid}", vname, got)
    site_vlans = set(exp["site_vlans"])
    for vid in sorted(set(obs["vlans"]) - set(exp["vlans"]), key=int):
        if int(vid) in site_vlans:
            differ(f"vlan {vid}", None, obs["vlans"][vid])

    want = set(exp["addresses"])
    got = {a for a in obs["addresses"] if in_model(a, exp["networks"])}
    for a in sorted(want | got):
        checked += 1
        if (a in want) != (a in got):
            differ(f"address {a}", a in want, a in got)

    for port, spec in exp["ports"].items():
        checked += 1
        if obs["ports"].get(port) != spec:
            differ(f"port {port}", spec, obs["ports"].get(port))
    return checked, diffs


# EOS stores well-known ports by name ("eq 22 443" reads back as
# "eq ssh https"); both sides are normalised to the same names.
EOS_PORT_NAMES = {"21": "ftp", "22": "ssh", "23": "telnet", "25": "smtp", "53": "domain", "67": "bootps",
                  "68": "bootpc", "80": "www", "123": "ntp", "161": "snmp", "443": "https"}


def normalise(line):
    return " ".join(EOS_PORT_NAMES.get(w, w) for w in line.split())


def config_lines(path):
    """Commands of a generated EOS configuration, normalised."""
    out = []
    for line in open(path, encoding="utf-8"):
        line = normalise(line)
        if line and not line.startswith("!") and line != "end":
            out.append(line)
    return out


def accepted(kind, obs, config):
    """Return (ok, detail): did the device take the whole configuration."""
    if kind == "arista_eos":
        running = {normalise(l) for l in obs["running"].splitlines()}
        missing = [l for l in config_lines(config) if l not in running]
        return not missing, {"missing": missing}
    errors = obs["script_errors"]
    return not errors and obs["vlan_filtering"], {"script_errors": errors, "vlan_filtering": obs["vlan_filtering"]}


def device_config(render, name):
    for root, _, files in os.walk(os.path.join(render, "configs")):
        for ext in (".cfg", ".rsc"):
            if name + ext in files:
                return os.path.join(root, name + ext)
    return None


def run(model_path, out_path, wait, render=None):
    exp = expected.load(model_path)
    creds = {
        "arista_eos": (os.environ.get("E4_EOS_USER", "admin"), os.environ.get("E4_EOS_PASSWORD", "admin")),
        "mikrotik": (os.environ.get("E4_ROS_USER", "admin"), os.environ.get("E4_ROS_PASSWORD", "admin")),
    }
    with open(model_path, encoding="utf-8") as f:
        model = expected.yaml.safe_load(f)
    site_vlans = {s["name"]: [n["vlan"] for n in s.get("networks", [])] for s in model["sites"]}

    report = {"model": os.path.basename(model_path), "devices": {}, "discrepancies": []}
    for name, dev in sorted(exp["devices"].items()):
        dev = dict(dev, site_vlans=site_vlans[dev["site"]])
        entry = {"type": dev["type"], "mgmt_ip": dev["mgmt_ip"]}
        started = time.monotonic()

        mgmt_ok = True
        for port in dev["management"]:
            ok = reachable(dev["mgmt_ip"], port, wait)
            mgmt_ok = mgmt_ok and ok
            if not ok:
                report["discrepancies"].append({"device": name, "parameter": f"management tcp/{port}",
                                                "expected": "reachable", "observed": "unreachable"})
        checked = len(dev["management"])

        if mgmt_ok:
            user, password = creds[dev["type"]]
            try:
                obs = observe.OBSERVERS[dev["type"]](dev["mgmt_ip"], user, password)
            except Exception as e:  # a device that cannot be read is one discrepancy
                report["discrepancies"].append({"device": name, "parameter": "read state",
                                                "expected": "readable", "observed": f"{type(e).__name__}: {e}"})
            else:
                n, diffs = compare_device(name, dev, obs)
                checked += n
                report["discrepancies"].extend(diffs)
                config = device_config(render, name) if render else None
                if config:
                    checked += 1
                    ok, detail = accepted(dev["type"], obs, config)
                    entry["accepted"] = ok
                    if not ok:
                        report["discrepancies"].append({"device": name, "parameter": "configuration accepted",
                                                        "expected": True, "observed": detail})
                obs.pop("running", None)
                entry["observed"] = obs
        entry["checked"] = checked
        entry["discrepancies"] = sum(1 for d in report["discrepancies"] if d["device"] == name)
        entry["seconds"] = round(time.monotonic() - started, 1)
        report["devices"][name] = entry

    if out_path:
        with open(out_path, "w", encoding="utf-8", newline="\n") as f:
            json.dump(report, f, ensure_ascii=False, indent=2, sort_keys=True)
            f.write("\n")

    print(f"{'пристрій':<16}{'тип':<12}{'параметрів':>11}{'розбіжностей':>14}  прийнято")
    for name, e in report["devices"].items():
        acc = {True: "так", False: "ні"}.get(e.get("accepted"), "—")
        print(f"{name:<16}{e['type']:<12}{e['checked']:>11}{e['discrepancies']:>14}  {acc}")
    for d in report["discrepancies"]:
        print(f"РОЗБІЖНІСТЬ {d['device']}: {d['parameter']}: очікувалось {d['expected']}, є {d['observed']}")
    return 1 if report["discrepancies"] else 0


if __name__ == "__main__":
    p = argparse.ArgumentParser(description="Звірка стану обладнання з моделлю")
    p.add_argument("model")
    p.add_argument("--out", help="JSON-звіт")
    p.add_argument("--wait", type=float, default=0, help="скільки секунд повторювати спроби досягти порту керування")
    p.add_argument("--render", help="вихід orgc render: перевірити, що пристрої прийняли конфігурацію повністю")
    a = p.parse_args()
    sys.exit(run(a.model, a.out, a.wait, a.render))
