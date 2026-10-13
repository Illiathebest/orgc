"""End-to-end check: expected state from the model against the devices.

Runs on the control node — in the emulation, inside the network namespace of
the server that stands in for it — so devices are read and management ports
probed along the path the model describes, through the site gateway and its
rules, not over containerlab's management network.

Every compared item is one parameter: a VLAN, an address, a port, a
management port reached. A discrepancy names the device, the parameter, and
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


def run(model_path, out_path, wait):
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
                entry["observed"] = obs
        entry["checked"] = checked
        entry["discrepancies"] = sum(1 for d in report["discrepancies"] if d["device"] == name)
        entry["seconds"] = round(time.monotonic() - started, 1)
        report["devices"][name] = entry

    if out_path:
        with open(out_path, "w", encoding="utf-8", newline="\n") as f:
            json.dump(report, f, ensure_ascii=False, indent=2, sort_keys=True)
            f.write("\n")

    print(f"{'пристрій':<16}{'тип':<12}{'параметрів':>11}{'розбіжностей':>14}")
    for name, e in report["devices"].items():
        print(f"{name:<16}{e['type']:<12}{e['checked']:>11}{e['discrepancies']:>14}")
    for d in report["discrepancies"]:
        print(f"РОЗБІЖНІСТЬ {d['device']}: {d['parameter']}: очікувалось {d['expected']}, є {d['observed']}")
    return 1 if report["discrepancies"] else 0


if __name__ == "__main__":
    p = argparse.ArgumentParser(description="Звірка стану обладнання з моделлю")
    p.add_argument("model")
    p.add_argument("--out", help="JSON-звіт")
    p.add_argument("--wait", type=float, default=0, help="скільки секунд повторювати спроби досягти порту керування")
    a = p.parse_args()
    sys.exit(run(a.model, a.out, a.wait))
