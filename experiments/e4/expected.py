"""Expected device state, derived from the model alone.

This is the oracle of the end-to-end check. It reads organization.yml
itself and applies the semantics documented in docs/data-model.md and the
port convention of ADR-009 — it does not use orgc or its output, so a defect
in orgc's lowering or rendering shows up as a discrepancy instead of being
copied into the expectation.

For every device it lists:
  vlans       VLAN ID -> network name, for every network that reaches the
              device: the networks on its ports, its mgmt network and, on
              the site's L3 gateway, every network with a gateway;
  addresses   "ip/len" on VLAN interfaces: mgmt_ip, and on the gateway the
              gateway address of every network;
  ports       trunk ports with the allowed VLANs and access ports with their
              VLAN; uplink ports are routed and not listed;
  management  TCP ports the control node must reach on mgmt_ip.
"""

import ipaddress
import json
import sys

import yaml

# Transport Ansible uses per platform (ADR-008): RouterOS API, SSH for EOS.
MANAGEMENT = {"mikrotik": [8728], "arista_eos": [22]}

# Star convention for a site without links (ADR-009, decision 3).
GATEWAY_TRUNKS = {"mikrotik": ["ether2", "ether3", "ether4"], "arista_eos": ["Ethernet2"]}
GATEWAY_UPLINK = {"mikrotik": "ether1", "arista_eos": "Ethernet1"}
SWITCH_TRUNK = {"mikrotik": "ether1", "arista_eos": "Ethernet1"}

L3_ROLES = {"edge_router", "firewall"}


def split_port(ref):
    device, port = ref.split(":", 1)
    return device, port


def network_of(networks, ip):
    addr = ipaddress.ip_address(ip)
    for n in networks:
        if addr in ipaddress.ip_network(n["cidr"], strict=False):
            return n
    return None


def site_expectation(site):
    networks = site.get("networks", [])
    by_name = {n["name"]: n for n in networks}
    devices = site.get("devices", [])
    gateway = next((d for d in devices if d["role"] in L3_ROLES), None)

    ports = {d["name"]: {} for d in devices}

    def put(device, port, value):
        # The first use of a port wins, as E002 allows only one.
        ports.setdefault(device, {}).setdefault(port, value)

    uplink_ports = set()
    for u in site.get("uplinks", []):
        if "port" in u:
            uplink_ports.add(split_port(u["port"]))
        elif gateway is not None:
            uplink_ports.add((gateway["name"], GATEWAY_UPLINK[gateway["type"]]))

    all_vlans = sorted(n["vlan"] for n in networks)
    links = site.get("links", [])
    if links:
        for link in links:
            vlans = sorted(by_name[name]["vlan"] for name in link["vlans"]) if "vlans" in link else all_vlans
            for end in (link["a"], link["b"]):
                device, port = split_port(end)
                put(device, port, {"mode": "trunk", "vlans": vlans})
    else:
        for d in devices:
            if d is gateway:
                for port in GATEWAY_TRUNKS[d["type"]]:
                    if (d["name"], port) not in uplink_ports:
                        put(d["name"], port, {"mode": "trunk", "vlans": all_vlans})
            else:
                put(d["name"], SWITCH_TRUNK[d["type"]], {"mode": "trunk", "vlans": all_vlans})

    for d in devices:
        for group in d.get("access_ports", []):
            for port in group["ports"]:
                put(d["name"], port, {"mode": "access", "vlan": by_name[group["network"]]["vlan"]})
    for s in site.get("servers", []):
        if "port" in s:
            device, port = split_port(s["port"])
            n = network_of(networks, s["ip"])
            put(device, port, {"mode": "access", "vlan": n["vlan"]})

    out = {}
    for d in devices:
        is_gw = d is gateway
        mine = ports.get(d["name"], {})
        vlan_ids = set()
        for p in mine.values():
            vlan_ids.update(p["vlans"] if p["mode"] == "trunk" else [p["vlan"]])
        mgmt = network_of(networks, d["mgmt_ip"])
        addresses = []
        for n in networks:
            bits = ipaddress.ip_network(n["cidr"], strict=False).prefixlen
            if is_gw and "gateway" in n:
                vlan_ids.add(n["vlan"])
                addresses.append(f"{n['gateway']}/{bits}")
            if n is mgmt:
                vlan_ids.add(n["vlan"])
                a = f"{d['mgmt_ip']}/{bits}"
                if a not in addresses:
                    addresses.append(a)
        out[d["name"]] = {
            "type": d["type"],
            "site": site["name"],
            "gateway": is_gw,
            "mgmt_ip": d["mgmt_ip"],
            "vlans": {str(n["vlan"]): n["name"] for n in networks if n["vlan"] in vlan_ids},
            "addresses": sorted(addresses),
            "ports": {p: v for p, v in sorted(mine.items()) if (d["name"], p) not in uplink_ports},
            "management": MANAGEMENT[d["type"]],
            "networks": [n["cidr"] for n in networks],
        }
    return out


def expectation(model):
    devices = {}
    for site in model["sites"]:
        devices.update(site_expectation(site))
    cn = model["control_node"]
    control = None
    for site in model["sites"]:
        for s in site.get("servers", []):
            if s["ip"] == cn["ip"]:
                control = s["name"]
    return {"control_node": {"ip": cn["ip"], "server": control}, "devices": devices}


def load(path):
    with open(path, encoding="utf-8") as f:
        return expectation(yaml.safe_load(f))


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("використання: expected.py <organization.yml>")
    json.dump(load(sys.argv[1]), sys.stdout, ensure_ascii=False, indent=2, sort_keys=True)
    sys.stdout.write("\n")
