package aristaeos

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"orgc/internal/ir"
	"orgc/internal/lower"
	"orgc/internal/model"
)

func testdata(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "..", "testdata"}, parts...)...)
}

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

func cfg(t *testing.T, m *ir.Model, device string) string {
	t.Helper()
	for _, d := range m.Devices {
		if d.Name == device {
			return string(Platform{}.Render(m, d)[0].Data)
		}
	}
	t.Fatalf("пристрою %s немає", device)
	return ""
}

func has(t *testing.T, c, block string) {
	t.Helper()
	if !strings.Contains(c, block) {
		t.Errorf("у конфігурації немає:\n%s", block)
	}
}

// A switch gets only the VLANs that reach it, trunks pruned as the links
// say, access ports in their network, and its management SVI.
func TestSwitch(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "m-profile.yml"))
	c := cfg(t, m, "acc-01")
	has(t, c, "vlan 10\n   name mgmt\nvlan 30\n   name servers\n!\n")
	if strings.Contains(c, "vlan 20\n") || strings.Contains(c, "vlan 40\n") {
		t.Error("на acc-01 не доходять users і guest — їхніх VLAN там бути не повинно")
	}
	has(t, c, "interface Ethernet1\n   description trunk to dist-01:Ethernet2\n   switchport mode trunk\n   switchport trunk allowed vlan 10,30\n")
	has(t, c, "interface Ethernet5\n   description server ctrl-01\n   switchport access vlan 30\n")
	has(t, c, "interface Vlan10\n   description mgmt\n   ip address 192.168.10.5/24\n")
	has(t, c, "ip route 0.0.0.0/0 192.168.10.1\n")
	if strings.Contains(c, "ip routing") || strings.Contains(c, "ip access-list orgc-in") {
		t.Error("комутатор не маршрутизує й не фільтрує між мережами")
	}
}

func TestClientIsolationIsAPortACL(t *testing.T) {
	c := cfg(t, lowerVariant(t, testdata("valid", "m-profile.yml")), "acc-03")
	has(t, c, "ip access-list orgc-isolate-guest\n   10 permit ip any host 192.168.40.1\n   20 permit udp any any eq bootps\n   30 deny ip any 192.168.40.0/24\n   40 permit ip any any\n")
	has(t, c, "interface Ethernet6\n   description access guest\n   switchport access vlan 40\n   ip access-group orgc-isolate-guest in\n")
	if strings.Contains(c, "interface Ethernet3\n   description access users\n   switchport access vlan 20\n   ip access-group") {
		t.Error("ACL ізоляції на порту неізольованої мережі")
	}
	if strings.Contains(c, "private-vlan") {
		t.Error("private VLAN не використовується")
	}
}

// EOS as the site's gateway: what EOS has is generated, what it lacks is
// named in a comment instead of being invented (ADR-009, decision 5).
func TestGateway(t *testing.T) {
	m := lowerVariant(t, testdata("broken", "w002-capability-missing.yml"),
		"    - { from: servers, to: mgmt, ports: [22, 8728], action: allow }\n",
		"    - { from: servers, to: mgmt, ports: [22, 8728], action: allow }\n    - { from: servers, to: internet, action: allow }\n",
		"cidr: 192.168.30.0/24, gateway: 192.168.30.1 }",
		"cidr: 192.168.30.0/24, gateway: 192.168.30.1,\n          dhcp_pool: { from: 192.168.30.100, to: 192.168.30.200 } }")
	c := cfg(t, m, "eos-01")
	has(t, c, "interface Ethernet1\n   description uplink (convention)\n   no switchport\n   ip address dhcp\n")
	has(t, c, "interface Vlan30\n   description servers\n   ip address 192.168.30.1/24\n   dhcp server ipv4\n   ip access-group orgc-in in\n")
	has(t, c, "ip routing\n")
	has(t, c, "dhcp server\n   subnet 192.168.30.0/24\n      range 192.168.30.100 192.168.30.200\n      default-gateway 192.168.30.1\n")
	has(t, c, "! W002: фільтрація зі станом недоступна")
	has(t, c, "! W002: трансляція адрес (NAT) недоступна")
	// The management port of the platform (SSH), open to the control node
	// network, before the final deny.
	has(t, c, "   20 permit tcp 192.168.30.0/24 host 192.168.10.1 eq 22\n")
	has(t, c, "permit tcp 192.168.30.0/24 192.168.10.0/24 eq 22 8728\n")
	if !regexp.MustCompile(`(?m)^   \d+ deny ip any any\n!\nend\n$`).MatchString(c) {
		t.Error("ACL має закінчуватися deny ip any any")
	}
}

func TestShapeAndDeterminism(t *testing.T) {
	m := lowerVariant(t, testdata("valid", "m-profile.yml"))
	for _, dev := range []string{"core-01", "dist-01", "acc-02", "core-br-01", "sw-branch-01"} {
		c := cfg(t, m, dev)
		if !strings.HasPrefix(c, "! Згенеровано orgc з m-profile.yml. Не редагувати вручну.\n") || !strings.HasSuffix(c, "end\n") {
			t.Errorf("%s: заголовок чи end", dev)
		}
		if strings.ContainsRune(c, '\r') {
			t.Errorf("%s: CR", dev)
		}
		for i := 0; i < 10; i++ {
			if cfg(t, m, dev) != c {
				t.Fatalf("%s: прогін %d дав іншу конфігурацію", dev, i)
			}
		}
	}
}
