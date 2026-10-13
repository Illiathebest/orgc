"""Mutation operators for the defect-detection experiment (E2).

Each operator takes a correct model and returns every mutant it can make
from it: a copy with exactly one defect of its class, and a short
description of what was changed. The class is the code orgc verify should
report for it. Operators work on the parsed YAML and never on text, so a
mutant differs from its base only where the defect is.

An operator that needs room — a free port, a free VLAN, an address outside
every pool — finds it in the model; where there is none, it yields nothing.
"""

import copy
import ipaddress

L3 = {"edge_router", "firewall"}
FREE_NET = "10.251.{}.0/24"


# --- reading the model ---------------------------------------------------

def gateway(site):
    return next((d for d in site.get("devices", []) if d["role"] in L3), None)


def mgmt_nets(site):
    return [n for n in site.get("networks", []) if n["role"] == "mgmt"]


def net_by_role(site, role):
    return [n for n in site.get("networks", []) if n["role"] == role]


def all_nets(m):
    return [n for s in m["sites"] for n in s.get("networks", [])]


def net_of(site, ip):
    a = ipaddress.ip_address(ip)
    for n in site.get("networks", []):
        if a in ipaddress.ip_network(n["cidr"], strict=False):
            return n
    return None


def used_ports(site):
    used = {}

    def use(ref):
        dev, port = ref.split(":", 1)
        used.setdefault(dev, set()).add(port)

    for l in site.get("links", []):
        use(l["a"])
        use(l["b"])
    for u in site.get("uplinks", []):
        if "port" in u:
            use(u["port"])
    for s in site.get("servers", []):
        if "port" in s:
            use(s["port"])
    for d in site.get("devices", []):
        for ap in d.get("access_ports", []):
            for p in ap["ports"]:
                used.setdefault(d["name"], set()).add(p)
    return used


def free_port(site, dev, taken=()):
    used = used_ports(site).get(dev["name"], set()) | set(taken)
    if dev["type"] == "mikrotik":
        # ether1 is the conventional uplink; start above it
        names = [f"ether{i}" for i in range(2, 11)]
    else:
        names = [f"Ethernet{i}" for i in range(1, 49)]
    return next((n for n in names if n not in used), None)


def static_ips(m):
    ips = {m["control_node"]["ip"]}
    for s in m["sites"]:
        ips |= {d["mgmt_ip"] for d in s.get("devices", [])}
        ips |= {v["ip"] for v in s.get("servers", [])}
        ips |= {n["gateway"] for n in s.get("networks", []) if "gateway" in n}
    return ips


def in_pool(net, ip):
    p = net.get("dhcp_pool")
    if not p:
        return False
    a = ipaddress.ip_address(ip)
    return ipaddress.ip_address(p["from"]) <= a <= ipaddress.ip_address(p["to"])


def free_ip(m, net):
    """A host address of the network that is not used and not in its pool."""
    taken = static_ips(m)
    for h in ipaddress.ip_network(net["cidr"], strict=False).hosts():
        ip = str(h)
        if ip not in taken and not in_pool(net, ip):
            return ip
    return None


def free_vlan(site):
    used = {n["vlan"] for n in site.get("networks", [])}
    return next(v for v in range(3001, 4095) if v not in used)


def names(m):
    out = {n["name"] for n in all_nets(m)}
    for s in m["sites"]:
        out |= {d["name"] for d in s.get("devices", [])}
        out |= {v["name"] for v in s.get("servers", [])}
    return out


def tree(site):
    """Parent of each device in the tree of active links, from the gateway."""
    gw = gateway(site)
    adj = {}
    for l in site.get("links", []):
        if l.get("standby"):
            continue
        a, b = l["a"].split(":")[0], l["b"].split(":")[0]
        adj.setdefault(a, []).append((b, l))
        adj.setdefault(b, []).append((a, l))
    parent, order = {gw["name"]: None}, [gw["name"]]
    for v in order:
        for w, l in adj.get(v, []):
            if w not in parent:
                parent[w] = (v, l)
                order.append(w)
    return parent, order


def path(site, dev):
    parent, _ = tree(site)
    out = []
    while parent.get(dev):
        dev, l = parent[dev]
        out.append(l)
    return out


def control_net(m):
    cn = m["control_node"]
    return cn["network"]


def rules(m):
    return m.setdefault("policies", {}).setdefault("inter_vlan", [])


def mutant(m, f):
    c = copy.deepcopy(m)
    f(c)
    return c


def sites_with_links(m):
    return [i for i, s in enumerate(m["sites"]) if s.get("links") and gateway(s)]


# --- operators by class ----------------------------------------------------

def e001(m):
    out = []
    for i, s in enumerate(m["sites"]):
        for n in s.get("networks", []):
            for k, sub in enumerate(("same", "half")):
                cidr = n["cidr"] if sub == "same" else str(next(ipaddress.ip_network(n["cidr"], strict=False).subnets(1)))

                def add(c, i=i, cidr=cidr):
                    site = c["sites"][i]
                    site["networks"].append({"vlan": free_vlan(site), "name": "mut-overlap", "role": "users", "cidr": cidr})
                out.append((f"нова мережа в сайті {s['name']} перетинається з {n['name']} ({cidr})", mutant(m, add)))
        for j, t in enumerate(m["sites"]):
            if j == i or t.get("allow_overlap") or s.get("allow_overlap"):
                continue
            for n in t.get("networks", [])[:2]:
                def add(c, i=i, cidr=n["cidr"]):
                    site = c["sites"][i]
                    site["networks"].append({"vlan": free_vlan(site), "name": "mut-overlap", "role": "users", "cidr": cidr})
                out.append((f"мережа сайту {s['name']} перетинається з {n['name']} сайту {t['name']}", mutant(m, add)))
    return out


def e002(m):
    out = []
    for i, s in enumerate(m["sites"]):
        nets = s.get("networks", [])
        for a in nets:
            for b in nets:
                if a is not b:
                    def dup(c, i=i, a=a["name"], v=b["vlan"]):
                        next(n for n in c["sites"][i]["networks"] if n["name"] == a)["vlan"] = v
                    out.append((f"VLAN {b['vlan']} мережі {b['name']} повторено в {a['name']}", mutant(m, dup)))
        for d in s.get("devices", []):
            srv_net = (net_by_role(s, "servers") or nets)[0]
            ip = free_ip(m, srv_net)
            if ip:
                def clash(c, i=i, name=d["name"], ip=ip):
                    c["sites"][i].setdefault("servers", []).append({"name": name, "role": ["app"], "ip": ip})
                out.append((f"сервер з іменем пристрою {d['name']}", mutant(m, clash)))
        ups = s.get("uplinks", [])
        if len(ups) > 1:
            def dupu(c, i=i):
                c["sites"][i]["uplinks"][1]["name"] = c["sites"][i]["uplinks"][0]["name"]
            out.append((f"два канали сайту {s['name']} з одним імʼям", mutant(m, dupu)))
        for l in s.get("links", []):
            dev, port = l["a"].split(":")
            if any(d["name"] == dev for d in s.get("devices", [])):
                net = (net_by_role(s, "users") or nets)[0]

                def reuse(c, i=i, dev=dev, port=port, net=net["name"]):
                    d = next(x for x in c["sites"][i]["devices"] if x["name"] == dev)
                    d.setdefault("access_ports", []).append({"ports": [port], "network": net})
                out.append((f"порт {dev}:{port} зʼєднання ще й access-порт", mutant(m, reuse)))
    for i, s in enumerate(m["sites"]):
        for j, t in enumerate(m["sites"]):
            if i == j:
                continue
            for n in t.get("networks", [])[:2]:
                def dupname(c, i=i, name=n["name"], k=len(out)):
                    site = c["sites"][i]
                    site["networks"].append({"vlan": free_vlan(site), "name": name, "role": "users", "cidr": FREE_NET.format(k % 250)})
                out.append((f"мережа {n['name']} повторена в сайті {s['name']}", mutant(m, dupname)))
    return out


def e003(m):
    out = []
    for r, rule in enumerate(m.get("policies", {}).get("inter_vlan", [])):
        for end in ("from", "to"):
            def ghost(c, r=r, end=end):
                c["policies"]["inter_vlan"][r][end] = "ghost-net"
            out.append((f"правило {r}: {end} на невідому мережу", mutant(m, ghost)))
    for i, s in enumerate(m["sites"]):
        for k, l in enumerate(s.get("links", [])):
            def ghostdev(c, i=i, k=k):
                port = c["sites"][i]["links"][k]["b"].split(":")[1]
                c["sites"][i]["links"][k]["b"] = "ghost-sw:" + port
            out.append((f"зʼєднання {l['a']} – {l['b']}: невідомий пристрій", mutant(m, ghostdev)))
            if "vlans" in l:
                def ghostvlan(c, i=i, k=k):
                    c["sites"][i]["links"][k]["vlans"].append("ghost-net")
                out.append((f"зʼєднання {l['a']} – {l['b']}: невідома мережа у vlans", mutant(m, ghostvlan)))
        for d in s.get("devices", []):
            for a, ap in enumerate(d.get("access_ports", [])):
                def ghostap(c, i=i, dev=d["name"], a=a):
                    next(x for x in c["sites"][i]["devices"] if x["name"] == dev)["access_ports"][a]["network"] = "ghost-net"
                out.append((f"access-порти {d['name']}: невідома мережа", mutant(m, ghostap)))
        gw = gateway(s)
        for d in s.get("devices", []):
            if d is gw or not s.get("uplinks"):
                continue
            p = free_port(s, d)
            if p:
                def upl(c, i=i, ref=f"{d['name']}:{p}"):
                    c["sites"][i]["uplinks"][0]["port"] = ref
                out.append((f"канал сайту {s['name']} на порту не-шлюзу {d['name']}", mutant(m, upl)))
    def ghostcn(c):
        c["control_node"]["network"] = "ghost-net"
    out.append(("вузол керування в невідомій мережі", mutant(m, ghostcn)))
    return out


def e004(m):
    out = []
    for i, s in enumerate(m["sites"]):
        gw = gateway(s)
        for d in s.get("devices", []):
            if d is gw:
                continue
            for n in s.get("networks", []):
                if n["role"] == "mgmt":
                    continue
                ip = free_ip(m, n)
                if ip:
                    def move(c, i=i, dev=d["name"], ip=ip):
                        next(x for x in c["sites"][i]["devices"] if x["name"] == dev)["mgmt_ip"] = ip
                    out.append((f"mgmt_ip {d['name']} у мережі {n['name']} ({n['role']})", mutant(m, move)))
    return out


def e005(m):
    nets = [n["name"] for n in all_nets(m)]
    out = [("правило any → any", mutant(m, lambda c: rules(c).append({"from": "any", "to": "any", "action": "allow"})))]
    for n in nets:
        for port in (443, 22):
            def inbound(c, n=n, port=port):
                rules(c).append({"from": "internet", "to": n, "ports": [port], "action": "allow"})
            out.append((f"правило internet → {n} на {port}", mutant(m, inbound)))
    return out


def platform_port(site):
    types = {d["type"] for d in site.get("devices", [])}
    return 8728 if "mikrotik" in types else 22


def e006(m):
    out = []
    cn = control_net(m)
    for r, rule in enumerate(m.get("policies", {}).get("inter_vlan", [])):
        to = [s for s in m["sites"] if any(n["name"] == rule["to"] and n["role"] == "mgmt" for n in s.get("networks", []))]
        if rule["from"] != cn or not to or "ports" not in rule:
            continue
        site = to[0]
        types = {d["type"] for d in site["devices"] if d.get("mgmt_ip") and net_of(site, d["mgmt_ip"])["name"] == rule["to"]}
        keep = [p for p in rule["ports"] if not ((p == 8728 and "mikrotik" in types) or (p == 22 and "arista_eos" in types))] or [443]
        def ports(c, r=r, keep=keep):
            c["policies"]["inter_vlan"][r]["ports"] = keep
        out.append((f"правило керування {rule['from']} → {rule['to']} без порту платформи: {keep}", mutant(m, ports)))
        def drop(c, r=r):
            del c["policies"]["inter_vlan"][r]
        out.append((f"правило керування {rule['from']} → {rule['to']} видалено", mutant(m, drop)))
    for i, s in enumerate(m["sites"]):
        mg = [n["name"] for n in mgmt_nets(s)]
        if not mg or not s.get("links"):
            continue
        for k, l in enumerate(s["links"]):
            if l.get("standby"):
                continue
            def prune(c, i=i, k=k, mg=mg):
                site = c["sites"][i]
                l = site["links"][k]
                l["vlans"] = [v for v in l.get("vlans", [n["name"] for n in site["networks"]]) if v not in mg]
            out.append((f"зʼєднання {l['a']} – {l['b']} без mgmt-мережі", mutant(m, prune)))
        for n in mgmt_nets(s):
            if "gateway" in n and n["name"] != cn:
                def unroute(c, i=i, name=n["name"]):
                    del next(x for x in c["sites"][i]["networks"] if x["name"] == name)["gateway"]
                out.append((f"mgmt-мережа {n['name']} без шлюзу", mutant(m, unroute)))
    return out


def e007(m):
    out = []
    for i, s in enumerate(m["sites"]):
        for n in s.get("networks", []):
            net = ipaddress.ip_network(n["cidr"], strict=False)
            if "dhcp_pool" in n and "gateway" in n:
                def gwpool(c, i=i, name=n["name"]):
                    x = next(y for y in c["sites"][i]["networks"] if y["name"] == name)
                    x["gateway"] = x["dhcp_pool"]["from"]
                out.append((f"шлюз {n['name']} у DHCP-пулі", mutant(m, gwpool)))
                def poolout(c, i=i, name=n["name"], to=str(net.broadcast_address + 5)):
                    next(y for y in c["sites"][i]["networks"] if y["name"] == name)["dhcp_pool"]["to"] = to
                out.append((f"кінець пулу {n['name']} поза мережею", mutant(m, poolout)))
                def inpool(c, i=i, ip=n["dhcp_pool"]["from"], k=len(out)):
                    c["sites"][i].setdefault("servers", []).append({"name": f"mut-srv{k}", "role": ["app"], "ip": ip})
                out.append((f"статична адреса сервера в пулі {n['name']}", mutant(m, inpool)))
            if "gateway" in n:
                def gwout(c, i=i, name=n["name"], gw=str(net.broadcast_address + 1)):
                    next(y for y in c["sites"][i]["networks"] if y["name"] == name)["gateway"] = gw
                out.append((f"шлюз {n['name']} поза мережею", mutant(m, gwout)))
        for d in s.get("devices", []):
            if d["mgmt_ip"] in {n.get("gateway") for n in s.get("networks", [])}:
                continue
            def collide(c, i=i, ip=d["mgmt_ip"], k=len(out)):
                c["sites"][i].setdefault("servers", []).append({"name": f"mut-srv{k}", "role": ["app"], "ip": ip})
            out.append((f"сервер з адресою {d['mgmt_ip']} пристрою {d['name']}", mutant(m, collide)))
    def cnout(c):
        c["control_node"]["ip"] = "203.0.113.10"
    out.append(("адреса вузла керування поза його мережею", mutant(m, cnout)))
    return out


def e008(m):
    """A routed network that cannot hold what it is given: a /31 or /32
    with a gateway, or a gateway plus a pool one address larger than the
    prefix has hosts."""
    out = []
    for i, s in enumerate(m["sites"]):
        if not gateway(s):
            continue
        for k, prefix in enumerate((31, 32, 30, 29, 28, 27)):
            def small(c, i=i, prefix=prefix, k=k):
                site = c["sites"][i]
                base = FREE_NET.format(200 + k)[: -len(".0/24")]
                n = {"vlan": free_vlan(site), "name": "mut-small", "role": "users", "cidr": f"{base}.0/{prefix}"}
                if prefix >= 31:
                    n["gateway"] = f"{base}.0"
                else:
                    hosts = 2 ** (32 - prefix) - 2
                    n["gateway"] = f"{base}.1"
                    n["dhcp_pool"] = {"from": f"{base}.2", "to": f"{base}.{2 + hosts}"}
                site["networks"].append(n)
            out.append((f"мережа /{prefix} у сайті {s['name']} замала для шлюзу й пулу", mutant(m, small)))
    return out


def e009(m):
    out = []
    for i in sites_with_links(m):
        s = m["sites"][i]
        parent, order = tree(s)
        leaves = {d for d in order if not any(p and p[0] == d for p in parent.values())}
        for k, l in enumerate(s["links"]):
            if l.get("standby") or len(s["links"]) == 1:
                # without its last link a site is a star by convention, where
                # every device is connected by definition: not a defect
                continue
            b = l["b"].split(":")[0]
            if b in leaves and not any(x.get("standby") and b in (x["a"].split(":")[0], x["b"].split(":")[0]) for x in s["links"]):
                def cut(c, i=i, k=k):
                    del c["sites"][i]["links"][k]
                out.append((f"зʼєднання до {b} видалено", mutant(m, cut)))
        for d in s.get("devices", []):
            for ap in d.get("access_ports", []):
                for l in path(s, d["name"])[:1]:
                    k = s["links"].index(l)
                    def prune(c, i=i, k=k, net=ap["network"]):
                        site = c["sites"][i]
                        x = site["links"][k]
                        x["vlans"] = [v for v in x.get("vlans", [n["name"] for n in site["networks"]]) if v != net]
                    out.append((f"зʼєднання {l['a']} – {l['b']} не несе {ap['network']} access-портів {d['name']}", mutant(m, prune)))
    return out


def e010(m):
    out = []
    for i in sites_with_links(m):
        s = m["sites"][i]
        parent, order = tree(s)
        devs = {d["name"]: d for d in s["devices"]}
        linked = {frozenset((l["a"].split(":")[0], l["b"].split(":")[0])) for l in s["links"]}
        for a in order:
            for b in order:
                if a >= b or frozenset((a, b)) in linked:
                    continue
                pa, pb = free_port(s, devs[a]), free_port(s, devs[b])
                if pa and pb:
                    def loop(c, i=i, ref=(f"{a}:{pa}", f"{b}:{pb}")):
                        c["sites"][i]["links"].append({"a": ref[0], "b": ref[1]})
                    out.append((f"зʼєднання {a} – {b} замикає петлю", mutant(m, loop)))
        for l in s["links"]:
            if l.get("standby"):
                continue
            a, b = l["a"].split(":")[0], l["b"].split(":")[0]
            pa, pb = free_port(s, devs[a]), free_port(s, devs[b])
            if pa and pb:
                def par(c, i=i, ref=(f"{a}:{pa}", f"{b}:{pb}")):
                    c["sites"][i]["links"].append({"a": ref[0], "b": ref[1]})
                out.append((f"друге активне зʼєднання {a} – {b}", mutant(m, par)))
        for d in order:
            p1 = free_port(s, devs[d])
            p2 = free_port(s, devs[d], taken={p1}) if p1 else None
            if p1 and p2:
                def selfl(c, i=i, ref=(f"{d}:{p1}", f"{d}:{p2}")):
                    c["sites"][i]["links"].append({"a": ref[0], "b": ref[1]})
                out.append((f"зʼєднання {d} сам із собою", mutant(m, selfl)))
    return out


def w001(m):
    out = []
    for i, s in enumerate(m["sites"]):
        for n in net_by_role(s, "guest"):
            def iso(c, i=i, name=n["name"]):
                next(x for x in c["sites"][i]["networks"] if x["name"] == name).pop("isolated", None)
            out.append((f"guest {n['name']} без isolated", mutant(m, iso)))
            for t in all_nets(m):
                if t["role"] in ("servers", "users") and t["name"] != n["name"]:
                    def rule(c, src=n["name"], dst=t["name"]):
                        rules(c).append({"from": src, "to": dst, "ports": [443], "action": "allow"})
                    out.append((f"правило {n['name']} → {t['name']}", mutant(m, rule)))
    return out


def w002(m):
    out = []
    for i, s in enumerate(m["sites"]):
        for d in s.get("devices", []):
            for ver in (("7.12", "5.0") if d["type"] == "mikrotik" else ("3.20.1F", "5.1.0F")):
                def bad(c, i=i, dev=d["name"], ver=ver):
                    next(x for x in c["sites"][i]["devices"] if x["name"] == dev)["os_version"] = ver
                out.append((f"{d['name']}: версія {ver} поза діапазоном", mutant(m, bad)))
        devs = {d["name"]: d for d in s.get("devices", [])}
        for k, l in enumerate(s.get("links", [])):
            dev = devs[l["a"].split(":")[0]]
            wrong = "Ethernet40" if dev["type"] == "mikrotik" else "eth40"
            def port(c, i=i, k=k, wrong=wrong):
                x = c["sites"][i]["links"][k]
                x["a"] = x["a"].split(":")[0] + ":" + wrong
            out.append((f"зʼєднання {l['a']}: імʼя порту {wrong} неприпустиме", mutant(m, port)))
    return out


def w003(m):
    out = []
    if m.get("profile") != "M":
        return out
    for i, s in enumerate(m["sites"]):
        if any(u["kind"] == "backup" for u in s.get("uplinks", [])):
            def drop(c, i=i):
                c["sites"][i]["uplinks"] = [u for u in c["sites"][i]["uplinks"] if u["kind"] != "backup"]
            out.append((f"сайт {s['name']} без резервного каналу", mutant(m, drop)))
    return out


def w004(m):
    out = []
    cn = control_net(m)
    for s in m["sites"]:
        for mg in mgmt_nets(s):
            for n in all_nets(m):
                if n["name"] in (cn, mg["name"]) or n["role"] == "mgmt":
                    continue
                for port in (22, 443):
                    def rule(c, src=n["name"], dst=mg["name"], port=port):
                        rules(c).append({"from": src, "to": dst, "ports": [port], "action": "allow"})
                    out.append((f"правило {n['name']} → {mg['name']} на {port}", mutant(m, rule)))
    return out


LEVEL = {"core_switch": 1, "distribution_switch": 2, "access_switch": 3}


def w005(m):
    """A switch that feeds others gets a role whose level is not above
    theirs: access feeding access, distribution feeding distribution..."""
    out = []
    for i in sites_with_links(m):
        s = m["sites"][i]
        parent, _ = tree(s)
        roles = {d["name"]: d["role"] for d in s["devices"]}
        for d in s["devices"]:
            kids = [k for k, p in parent.items() if p and p[0] == d["name"]]
            if not kids or d["role"] not in LEVEL:
                continue
            lowest = min(LEVEL.get(roles[k], 0) for k in kids)
            for role, level in LEVEL.items():
                if role == d["role"] or level < lowest:
                    continue
                def rerole(c, i=i, dev=d["name"], role=role):
                    next(x for x in c["sites"][i]["devices"] if x["name"] == dev)["role"] = role
                out.append((f"{d['name']}: роль {role}, а живить {', '.join(kids)}", mutant(m, rerole)))
    return out


def w006(m):
    out = []
    for i in sites_with_links(m):
        s = m["sites"][i]
        mg = [n["name"] for n in mgmt_nets(s)]
        for k, l in enumerate(s["links"]):
            if not l.get("standby"):
                continue
            def prune(c, i=i, k=k, mg=mg):
                site = c["sites"][i]
                x = site["links"][k]
                x["vlans"] = [v for v in x.get("vlans", [n["name"] for n in site["networks"]]) if v not in mg]
            out.append((f"резервне зʼєднання {l['a']} – {l['b']} без mgmt-мережі", mutant(m, prune)))
    return out


OPERATORS = {
    "E001": e001, "E002": e002, "E003": e003, "E004": e004, "E005": e005,
    "E006": e006, "E007": e007, "E008": e008, "E009": e009, "E010": e010,
    "W001": w001, "W002": w002, "W003": w003, "W004": w004, "W005": w005, "W006": w006,
}
