"""Observed device state, read with NAPALM.

The result has the shape of expected.py, so the two compare key by key:
vlans (ID -> name), addresses ("ip/len"), ports (trunk with VLANs or access
with its VLAN). Besides, it carries what shows whether the device took the
whole generated configuration: the running configuration of EOS, the script
errors in the RouterOS log and whether VLAN filtering is on. Addresses outside the model's networks — the containerlab
management interface, the uplink — are left out by compare.py, not here.

EOS is read with the eos driver over SSH (napalm >= 4.1), so eAPI need not
be enabled. RouterOS is read with napalm-ros; the driver has no VLAN getter,
so the bridge tables come from its RouterOS API connection.
"""

import json

from napalm import get_network_driver


def parse_vlan_list(text):
    """Parse an EOS VLAN list such as "10,20-22" into sorted IDs."""
    if text in ("", "NONE"):
        return []
    if text == "ALL":
        return list(range(1, 4095))
    out = []
    for part in text.split(","):
        if "-" in part:
            lo, hi = part.split("-")
            out.extend(range(int(lo), int(hi) + 1))
        else:
            out.append(int(part))
    return sorted(out)


def addresses(driver):
    out = []
    for ifc in driver.get_interfaces_ip().values():
        for addr, info in ifc.get("ipv4", {}).items():
            out.append(f"{addr}/{info['prefix_length']}")
    return sorted(out)


def observe_eos(host, user, password, timeout=30):
    driver = get_network_driver("eos")(host, user, password, timeout=timeout, optional_args={"transport": "ssh"})
    driver.open()
    try:
        vlans = {str(vid): v["name"] for vid, v in driver.get_vlans().items() if int(vid) != 1}
        cmd = "show interfaces switchport | json"
        switchport = json.loads(driver.cli([cmd])[cmd])
        ports = {}
        for name, sp in switchport.get("switchports", {}).items():
            info = sp.get("switchportInfo", {})
            if info.get("mode") == "trunk":
                ports[name] = {"mode": "trunk", "vlans": parse_vlan_list(info.get("trunkAllowedVlans", ""))}
            elif info.get("mode") == "access":
                ports[name] = {"mode": "access", "vlan": info.get("accessVlanId")}
        running = driver.get_config(retrieve="running")["running"]
        return {"vlans": vlans, "addresses": addresses(driver), "ports": ports, "running": running}
    finally:
        driver.close()


def split(value):
    if not value:
        return []
    if isinstance(value, (list, tuple)):
        return list(value)
    return [v for v in str(value).split(",") if v]


def observe_ros(host, user, password, timeout=30):
    driver = get_network_driver("ros")(host, user, password, timeout=timeout)
    driver.open()
    try:
        api = driver.api
        table = [v for v in api.path("interface", "bridge", "vlan") if not v.get("dynamic")]
        vlans = {}
        tagged = {}
        for v in table:
            for vid in split(v.get("vlan-ids")):
                vlans[str(vid)] = v.get("comment", "")
                for port in split(v.get("tagged")):
                    tagged.setdefault(port, []).append(int(vid))
        ports = {}
        for p in api.path("interface", "bridge", "port"):
            if p.get("dynamic"):
                continue
            name = p["interface"]
            if p.get("frame-types") == "admit-only-vlan-tagged":
                ports[name] = {"mode": "trunk", "vlans": sorted(tagged.get(name, []))}
            else:
                ports[name] = {"mode": "access", "vlan": int(p.get("pvid", 1))}
        # /import stops at the first failing command and logs it; the script
        # switches VLAN filtering on as its very last command.
        errors = [e.get("message", "") for e in api.path("log") if "script" in e.get("topics", "") and "error" in e.get("topics", "")]
        filtering = [str(b.get("vlan-filtering")).lower() in ("true", "yes") for b in api.path("interface", "bridge")]
        return {"vlans": vlans, "addresses": addresses(driver), "ports": ports,
                "script_errors": errors, "vlan_filtering": bool(filtering) and all(filtering)}
    finally:
        driver.close()


OBSERVERS = {"arista_eos": observe_eos, "mikrotik": observe_ros}


if __name__ == "__main__":
    import sys

    if len(sys.argv) != 5:
        sys.exit("використання: observe.py <arista_eos|mikrotik> <адреса> <користувач> <пароль>")
    kind, host, user, password = sys.argv[1:]
    json.dump(OBSERVERS[kind](host, user, password), sys.stdout, ensure_ascii=False, indent=2, sort_keys=True)
    sys.stdout.write("\n")
