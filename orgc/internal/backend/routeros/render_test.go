package routeros

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orgc/internal/ir"
	"orgc/internal/lower"
	"orgc/internal/model"
)

func testdata(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "..", "testdata"}, parts...)...)
}

// lowerSrc lowers a model given as text, after applying edits to a fixture.
func lowerVariant(t *testing.T, path string, edits ...string) *ir.Model {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for i := 0; i+1 < len(edits); i += 2 {
		if n := strings.Count(s, edits[i]); n != 1 {
			t.Fatalf("фрагмент %q трапляється %d разів", edits[i], n)
		}
		s = strings.Replace(s, edits[i], edits[i+1], 1)
	}
	doc, ds, err := model.LoadBytes(filepath.Base(path), []byte(s))
	if err != nil || doc == nil {
		t.Fatalf("err = %v, ds = %v", err, ds)
	}
	m, err := lower.Lower(doc)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func rsc(t *testing.T, m *ir.Model, device string) string {
	t.Helper()
	for _, d := range m.Devices {
		if d.Name == device {
			files := Platform{}.Render(m, d)
			if len(files) != 1 {
				t.Fatalf("файлів %d", len(files))
			}
			return string(files[0].Data)
		}
	}
	t.Fatalf("пристрою %s немає", device)
	return ""
}

func has(t *testing.T, script, line string) {
	t.Helper()
	if !strings.Contains(script, line+"\n") {
		t.Errorf("у скрипті немає рядка:\n  %s", line)
	}
}

// The management port stays open to the control node network even when no
// rule is needed for it: with the control node in the mgmt network itself,
// E006 holds without a rule, and the input chain must agree.
func TestManagementAlwaysOpen(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "lab.yml"))
	s := rsc(t, m, "rb750")
	has(t, s, `add chain=input action=accept src-address=192.168.1.0/24 dst-address=192.168.1.220 protocol=tcp dst-port=8728 comment="management from lan"`)
	if strings.Index(s, "management from lan") > strings.Index(s, `chain=input action=drop comment="default deny"`) {
		t.Error("дозвіл керування стоїть після default deny")
	}
}

// A DHCP client already on an uplink port — CHR has one on ether1 out of the
// box — is removed before the uplink's own is added, so the import does not
// stop halfway on "already exists".
func TestUplinkReplacesExistingDHCPClient(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "s-profile.yml"))
	s := rsc(t, m, "rb750")
	remove := strings.Index(s, "remove [find interface=ether1]\n")
	add := strings.Index(s, "add interface=ether1 add-default-route=yes")
	if remove < 0 || add < 0 || remove > add {
		t.Errorf("remove має стояти перед add у /ip dhcp-client:\n%s", s)
	}
}

// A site without links follows the convention: ether1 uplink, ether2–ether4
// trunk; ether5, the emergency WinBox port of RB750, is left alone.
func TestStarConvention(t *testing.T) {
	s := rsc(t, lowerVariant(t, testdata("valid", "lab.yml")), "rb750")
	for _, p := range []string{"ether2", "ether3", "ether4"} {
		has(t, s, "add bridge=bridge interface="+p+` frame-types=admit-only-vlan-tagged ingress-filtering=yes comment="trunk (convention)"`)
	}
	has(t, s, "add list=WAN interface=ether1")
	if strings.Contains(s, "ether5") {
		t.Error("ether5 не має згадуватися: незгадані порти не налаштовуються")
	}
}

func TestIsolatedAccessPortsShareAHorizon(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "s-profile.yml"),
		"        mgmt_ip: 192.168.10.1\n",
		"        mgmt_ip: 192.168.10.1\n        access_ports:\n          - { ports: [ether3, ether4], network: guest }\n")
	s := rsc(t, m, "rb750")
	has(t, s, `add bridge=bridge interface=ether3 pvid=40 frame-types=admit-only-untagged-and-priority-tagged ingress-filtering=yes horizon=40 comment="access guest"`)
	has(t, s, "add bridge=bridge vlan-ids=40 tagged=bridge,ether2 untagged=ether3,ether4 comment=guest")
}

// A gateway enforces the rules that touch its site; the policy is global,
// but the rules of another site never pass through it.
func TestGatewayGetsRulesOfItsSite(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "m-profile.yml"))
	branch := rsc(t, m, "rb-branch")
	if strings.Contains(branch, `comment="users -> servers"`) {
		t.Error("правило HQ users -> servers потрапило на шлюз філії")
	}
	has(t, branch, `add chain=forward action=accept src-address=192.168.30.0/24 dst-address=192.168.110.0/24 protocol=tcp dst-port=22,8728 comment="servers -> mgmt-branch"`)
	hq := rsc(t, m, "rb750")
	has(t, hq, `add chain=forward action=accept src-address=192.168.30.0/24 dst-address=192.168.110.0/24 protocol=tcp dst-port=22,8728 comment="servers -> mgmt-branch"`)
}

func TestAnyExpandsToModelNetworks(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "s-profile.yml"),
		"    - { from: users, to: servers, ports: [443, 22], action: allow }\n",
		"    - { from: users, to: any, ports: [443], action: allow }\n")
	s := rsc(t, m, "rb750")
	has(t, s, `add chain=forward action=accept src-address=192.168.20.0/24 dst-address-list=orgc-networks protocol=tcp dst-port=443 comment="users -> any"`)
	// to: any opens every address of the gateway in the model's networks.
	has(t, s, `add chain=input action=accept src-address=192.168.20.0/24 dst-address=192.168.10.1 protocol=tcp dst-port=443 comment="users -> any"`)
	has(t, s, "add list=orgc-networks address=192.168.40.0/24 comment=guest")
}

func TestMikroTikAsSwitch(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "minimal.yml"),
		"        mgmt_ip: 192.168.10.1\n",
		"        mgmt_ip: 192.168.10.1\n      - { name: sw-01, type: mikrotik, os_version: \"6.48\", role: access_switch, mgmt_ip: 192.168.10.2 }\n")
	s := rsc(t, m, "sw-01")
	has(t, s, `add bridge=bridge interface=ether1 frame-types=admit-only-vlan-tagged ingress-filtering=yes comment="trunk to gateway (convention)"`)
	has(t, s, "add address=192.168.10.2/24 interface=vlan10-mgmt comment=mgmt")
	has(t, s, "add gateway=192.168.10.1 comment=orgc")
	if strings.Contains(s, "/ip firewall") || strings.Contains(s, "dhcp-server") {
		t.Error("комутатор не фільтрує й не роздає адрес")
	}
}

// RouterOS 6 handles only ASCII reliably; the bridge is created without
// filtering and filtering is the very last command.
func TestScriptShape(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "m-profile.yml"))
	for _, dev := range []string{"rb750", "rb-branch"} {
		s := rsc(t, m, dev)
		for i, r := range s {
			if r > 127 {
				t.Fatalf("%s: не-ASCII символ %q на позиції %d", dev, r, i)
			}
		}
		if strings.ContainsRune(s, '\r') {
			t.Errorf("%s: CR у скрипті", dev)
		}
		if !strings.Contains(s, "add name=bridge vlan-filtering=no") || !strings.HasSuffix(s, "set [find name=bridge] vlan-filtering=yes\n") {
			t.Errorf("%s: VLAN-фільтрація має вмикатися останньою командою", dev)
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "m-profile.yml"))
	first := rsc(t, m, "rb750")
	for i := 0; i < 20; i++ {
		if rsc(t, m, "rb750") != first {
			t.Fatalf("прогін %d дав інший скрипт", i)
		}
	}
}

// RSTP on every bridge: the gateway is the root, any other device keeps
// the default priority, and a standby link's port carries the highest
// cost, so that STP blocks it and the active topology is the verified tree.
func TestRSTP(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "s-profile.yml"),
		"      - { a: core-01:Ethernet3, b: acc-02:Ethernet1 }\n",
		"      - { a: core-01:Ethernet3, b: acc-02:Ethernet1 }\n      - { a: rb750:ether3, b: acc-01:Ethernet2, standby: true }\n")
	s := rsc(t, m, "rb750")
	has(t, s, "add name=bridge vlan-filtering=no protocol-mode=rstp priority=0x1000 comment=orgc")
	has(t, s, `add bridge=bridge interface=ether3 frame-types=admit-only-vlan-tagged ingress-filtering=yes path-cost=200000000 comment="standby trunk to acc-01:Ethernet2"`)
	has(t, s, `add bridge=bridge interface=ether2 frame-types=admit-only-vlan-tagged ingress-filtering=yes comment="trunk to core-01:Ethernet1"`)

	sw := rsc(t, lowerVariant(t, testdata("valid", "minimal.yml"),
		"        mgmt_ip: 192.168.10.1\n",
		"        mgmt_ip: 192.168.10.1\n      - { name: sw-01, type: mikrotik, os_version: \"6.48\", role: access_switch, mgmt_ip: 192.168.10.2 }\n"), "sw-01")
	has(t, sw, "add name=bridge vlan-filtering=no protocol-mode=rstp priority=0x8000 comment=orgc")
}
