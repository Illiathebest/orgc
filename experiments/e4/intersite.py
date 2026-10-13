"""Emulation scaffolding: the inter-site link the model assumes.

The model does not describe how sites reach one another: it assumes the L3
gateways are fully meshed (in the lab, a WireGuard tunnel). orgc renders
nothing for it, and in containerlab nothing provides it — RouterOS under
vrnetlab sits behind QEMU's NAT on the management network, and a gateway's
default route leads to its masqueraded uplink.

This script stands in for the tunnel. It copies the topology orgc rendered
into <out>/lab/, adds a link between the gateways of every pair of sites on
their first free ports, and points each gateway's startup config at a copy
of orgc's configuration with a few lines appended: a /30 transit address
from 10.99.2.0/24 and static routes to the other sites' networks. Nothing
of orgc's own output is changed, so the check of whether a device took
orgc's whole configuration still compares against it.

    intersite.py <organization.yml> <render dir> <out dir>
"""

import itertools
import os
import shutil
import sys

import yaml

L3_ROLES = {"edge_router", "firewall"}
TRANSIT = "10.99.2"
KIND = {"mikrotik": "mikrotik_ros", "arista_eos": "arista_ceos"}


def port_name(kind, ifc):
    """Device port for a container interface: vrnetlab RouterOS keeps
    ether1 for management, so eth1 is ether2; cEOS ethN is EthernetN."""
    n = int(ifc[3:])
    return f"ether{n + 1}" if kind == "mikrotik_ros" else f"Ethernet{n}"


def used_interfaces(topo):
    used = {}
    for link in topo.get("links", []):
        ends = link.get("endpoints") or [f"{link['endpoint']['node']}:{link['endpoint']['interface']}"]
        for e in ends:
            node, ifc = e.split(":", 1)
            used.setdefault(node, set()).add(ifc)
    return used


def free_interface(used):
    n = 1
    while f"eth{n}" in used:
        n += 1
    return f"eth{n}"


def snippet(kind, port, address, routes):
    if kind == "mikrotik_ros":
        lines = ["", "# Emulation only: inter-site link standing in for the tunnel the model assumes.",
                 "/ip address", f"add address={address}/30 interface={port} comment=e4-intersite", "/ip route"]
        lines += [f"add dst-address={dst} gateway={via} comment=e4-intersite" for dst, via in routes]
    else:
        lines = ["! Emulation only: inter-site link standing in for the tunnel the model assumes.",
                 f"interface {port}", "   no switchport", f"   ip address {address}/30", "!"]
        lines += [f"ip route {dst} {via}" for dst, via in routes]
    return "\n".join(lines) + "\n"


def main(model_path, render, out):
    with open(model_path, encoding="utf-8") as f:
        model = yaml.safe_load(f)
    with open(os.path.join(render, "containerlab", "lab.clab.yml"), encoding="utf-8") as f:
        topo = yaml.safe_load(f)

    lab = os.path.join(out, "lab")
    shutil.rmtree(lab, ignore_errors=True)
    os.makedirs(lab)
    nodes = topo["topology"]["nodes"]
    for node in nodes.values():
        if "startup-config" in node:
            node["startup-config"] = os.path.join("..", "render", "configs", node["startup-config"].split("configs/", 1)[1])

    gateways = []
    for site in model["sites"]:
        gw = next((d for d in site.get("devices", []) if d["role"] in L3_ROLES), None)
        if gw and gw["name"] in nodes:
            gateways.append((site, gw))

    used = used_interfaces(topo["topology"])
    extra = {}
    links = topo["topology"].setdefault("links", [])
    for i, ((sa, ga), (sb, gb)) in enumerate(itertools.combinations(gateways, 2)):
        ends = []
        for g in (ga, gb):
            ifc = free_interface(used.setdefault(g["name"], set()))
            used[g["name"]].add(ifc)
            ends.append(ifc)
        links.append({"endpoints": [f"{ga['name']}:{ends[0]}", f"{gb['name']}:{ends[1]}"]})
        addr = {ga["name"]: f"{TRANSIT}.{4 * i + 1}", gb["name"]: f"{TRANSIT}.{4 * i + 2}"}
        for (s, g, ifc), (peer_site, peer) in (((sa, ga, ends[0]), (sb, gb)), ((sb, gb, ends[1]), (sa, ga))):
            kind = KIND[g["type"]]
            routes = [(n["cidr"], addr[peer["name"]]) for n in peer_site.get("networks", [])]
            extra.setdefault(g["name"], []).append(snippet(kind, port_name(kind, ifc), addr[g["name"]], routes))
            print(f"канал між сайтами: {g['name']}:{port_name(kind, ifc)} {addr[g['name']]}/30 → {peer['name']}")

    for name, parts in extra.items():
        node = nodes[name]
        src = os.path.join(lab, node["startup-config"])
        dst = name + os.path.splitext(src)[1]
        with open(src, encoding="utf-8") as f:
            text = f.read()
        if dst.endswith(".cfg") and text.rstrip().endswith("end"):
            text = text.rstrip()[: -len("end")]
            parts = parts + ["end\n"]
        with open(os.path.join(lab, dst), "w", encoding="utf-8", newline="\n") as f:
            f.write(text + "".join(parts))
        node["startup-config"] = dst

    with open(os.path.join(lab, "lab.clab.yml"), "w", encoding="utf-8", newline="\n") as f:
        yaml.safe_dump(topo, f, allow_unicode=True, sort_keys=False)


if __name__ == "__main__":
    if len(sys.argv) != 4:
        sys.exit("використання: intersite.py <organization.yml> <каталог render> <каталог виходу>")
    main(*sys.argv[1:])
