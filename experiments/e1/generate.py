"""Synthetic organization models for the scaling experiment (E1).

A model is built from identical sites of ten devices — an L3 gateway, a core
switch, two distribution and six access switches, every access switch with a
standby link up to the other distribution — so the device count is ten times
the site count. Gateways alternate between MikroTik and Arista EOS. The
control node sits in the servers network of the first site.

Each site has the networks mgmt, users, servers and guest (isolated), plus
--extra-networks routed ones that only the gateway and the core carry. The
policy is what a real organization writes: management of every site from
the control node's network, users to their servers and to the internet,
guests to the internet only, users of every site to the central servers;
--extra-rules adds rules between users and servers of random site pairs.

Every model passes orgc verify with no errors: the only findings are W002
for EOS gateways (no NAT, no stateful filtering). The output is a pure
function of the arguments: the same seed gives the same bytes.

    generate.py --devices 200 [--extra-networks N] [--extra-rules N] [--seed S]
"""

import argparse
import random
import sys

SITE_DEVICES = 10
MIKROTIK = ("mikrotik", "6.48")
EOS = ("arista_eos", "4.36.2F")


def site(i, extra_networks):
    s = f"s{i:03d}"
    gw_type, gw_ver = MIKROTIK if i % 2 == 0 else EOS
    if gw_type == "mikrotik":
        up, backup, to_core = "ether1", "ether5", "ether2"
    else:
        up, backup, to_core = "Ethernet1", "Ethernet8", "Ethernet2"

    base = ["mgmt", "users", "servers", "guest"]
    nets = []
    for n, role in enumerate(base + [f"lab{k}" for k in range(1, extra_networks + 1)]):
        cidr = f"10.{i}.{n}.0/24"
        name = f"{role}-{s}"
        net = {"vlan": 10 * (n + 1), "name": name, "role": role if role in base else "users",
               "cidr": cidr, "gateway": f"10.{i}.{n}.1"}
        if role in ("users", "guest") or role.startswith("lab"):
            net["dhcp_pool"] = {"from": f"10.{i}.{n}.100", "to": f"10.{i}.{n}.200"}
        if role == "guest":
            net["isolated"] = True
        nets.append(net)

    gw = f"gw-{s}"
    core = f"core-{s}"
    dist = [f"dist1-{s}", f"dist2-{s}"]
    acc = [f"acc{k}-{s}" for k in range(1, 7)]
    devices = [{"name": gw, "type": gw_type, "os_version": gw_ver, "role": "edge_router", "mgmt_ip": f"10.{i}.0.1"}]
    roles = [(core, "core_switch")] + [(d, "distribution_switch") for d in dist] + [(a, "access_switch") for a in acc]
    for k, (name, role) in enumerate(roles):
        dev = {"name": name, "type": EOS[0], "os_version": EOS[1], "role": role, "mgmt_ip": f"10.{i}.0.{k + 2}"}
        if role == "access_switch":
            dev["access_ports"] = [{"ports": ["Ethernet3", "Ethernet4"], "network": f"users-{s}"},
                                   {"ports": ["Ethernet6"], "network": f"guest-{s}"}]
        devices.append(dev)

    edge = [f"mgmt-{s}", f"users-{s}", f"servers-{s}", f"guest-{s}"]
    links = [
        {"a": f"{gw}:{to_core}", "b": f"{core}:Ethernet1"},
        {"a": f"{core}:Ethernet2", "b": f"{dist[0]}:Ethernet1"},
        {"a": f"{core}:Ethernet3", "b": f"{dist[1]}:Ethernet1"},
    ]
    for k, a in enumerate(acc):
        own, other = (dist[0], dist[1]) if k < 3 else (dist[1], dist[0])
        links.append({"a": f"{own}:Ethernet{2 + k % 3}", "b": f"{a}:Ethernet1", "vlans": edge})
        links.append({"a": f"{other}:Ethernet{5 + k % 3}", "b": f"{a}:Ethernet2", "vlans": edge, "standby": True})

    servers = [{"name": f"srv-{s}", "role": ["docker_host"], "ip": f"10.{i}.2.10", "port": f"{acc[0]}:Ethernet5"}]
    return {
        "name": s,
        "uplinks": [{"name": f"isp-{s}", "kind": "primary", "port": f"{gw}:{up}"},
                    {"name": f"lte-{s}", "kind": "backup", "port": f"{gw}:{backup}"}],
        "networks": nets,
        "devices": devices,
        "servers": servers,
        "links": links,
    }


def model(devices, extra_networks, extra_rules, seed):
    if devices % SITE_DEVICES:
        sys.exit(f"кількість пристроїв має бути кратною {SITE_DEVICES}")
    count = devices // SITE_DEVICES
    sites = [site(i, extra_networks) for i in range(count)]
    control = "servers-s000"
    rules = []
    for i in range(count):
        s = f"s{i:03d}"
        rules += [
            {"from": control, "to": f"mgmt-{s}", "ports": [22, 8728], "action": "allow"},
            {"from": f"users-{s}", "to": f"servers-{s}", "ports": [443], "action": "allow"},
            {"from": f"users-{s}", "to": "internet", "action": "allow"},
            {"from": f"guest-{s}", "to": "internet", "action": "allow"},
        ]
        if i:
            rules.append({"from": f"users-{s}", "to": control, "ports": [443], "action": "allow"})
    rng = random.Random(seed)
    seen = {(r["from"], r["to"]) for r in rules}
    while extra_rules and count > 1:
        a, b = rng.sample(range(count), 2)
        pair = (f"users-s{a:03d}", f"servers-s{b:03d}")
        if pair in seen:
            if len(seen) >= count * (count - 1) + 5 * count:
                break
            continue
        seen.add(pair)
        rules.append({"from": pair[0], "to": pair[1], "ports": [rng.choice([22, 443, 5432, 8080])], "action": "allow"})
        extra_rules -= 1
    return {
        "organization": f"Синтетична модель: {devices} пристроїв",
        "profile": "M",
        "control_node": {"site": "s000", "network": control, "ip": "10.0.2.10"},
        "sites": sites,
        "policies": {"inter_vlan": rules},
    }


def flow(value):
    """A YAML flow value: lists and maps inline, strings bare when safe."""
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    if isinstance(value, list):
        return "[" + ", ".join(flow(v) for v in value) + "]"
    if isinstance(value, dict):
        return "{ " + ", ".join(f"{k}: {flow(v)}" for k, v in value.items()) + " }"
    if value[:1].isdigit() or any(c in value for c in ":#{}[],&*!|>'\"%@`") or value in ("true", "false"):
        return '"' + value + '"'
    return value


def dump(m):
    out = ["---", f"# Згенеровано experiments/e1/generate.py. {m['organization']}.",
           f"organization: \"{m['organization']}\"", f"profile: {m['profile']}", "",
           f"control_node: {flow(m['control_node'])}", "", "sites:"]
    for s in m["sites"]:
        out.append(f"  - name: {s['name']}")
        for key in ("uplinks", "networks", "devices", "servers", "links"):
            out.append(f"    {key}:")
            out += [f"      - {flow(x)}" for x in s[key]]
    out += ["", "policies:", "  inter_vlan:"]
    out += [f"    - {flow(r)}" for r in m["policies"]["inter_vlan"]]
    return "\n".join(out) + "\n"


if __name__ == "__main__":
    p = argparse.ArgumentParser(description="Синтетична модель для E1")
    p.add_argument("--devices", type=int, required=True)
    p.add_argument("--extra-networks", type=int, default=0)
    p.add_argument("--extra-rules", type=int, default=0)
    p.add_argument("--seed", type=int, default=1)
    a = p.parse_args()
    sys.stdout.reconfigure(newline="\n", encoding="utf-8")
    sys.stdout.write(dump(model(a.devices, a.extra_networks, a.extra_rules, a.seed)))
