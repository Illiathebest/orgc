package lower

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orgc/internal/ir"
	"orgc/internal/model"
)

func testdata(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...)
}

// load parses a fixture and fails the test unless it passes the schema:
// lowering only ever sees structurally valid models.
func load(t *testing.T, path string) *model.Document {
	t.Helper()
	doc, ds, err := model.Load(path)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	if doc == nil {
		t.Fatalf("Load(%s): модель не пройшла схему: %v", path, ds)
	}
	return doc
}

// loadVariant applies textual edits to a fixture and parses the result.
// Each pair is old, new; every old string must occur exactly once, so that a
// fixture edit cannot silently turn a test into a no-op.
func loadVariant(t *testing.T, path string, edits ...string) *model.Document {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for i := 0; i+1 < len(edits); i += 2 {
		if n := strings.Count(s, edits[i]); n != 1 {
			t.Fatalf("фрагмент %q трапляється %d разів, want 1", edits[i], n)
		}
		s = strings.Replace(s, edits[i], edits[i+1], 1)
	}
	doc, ds, err := model.LoadBytes(filepath.Base(path), []byte(s))
	if err != nil || doc == nil {
		t.Fatalf("варіант не пройшов схему: err = %v, ds = %v", err, ds)
	}
	return doc
}

func lower(t *testing.T, doc *model.Document) *ir.Model {
	t.Helper()
	m, err := Lower(doc)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return m
}

func validFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(testdata("valid", "*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("у testdata/valid немає моделей: %v", err)
	}
	return files
}

// On a valid model everything resolves, every address parses, and every
// element can be traced back to a line — the three things the invariants
// take for granted.
func TestValidCorpusLowersCompletely(t *testing.T) {
	for _, f := range validFiles(t) {
		t.Run(filepath.Base(f), func(t *testing.T) {
			m := lower(t, load(t, f))

			if len(m.Sites) == 0 || len(m.Networks) == 0 || len(m.Devices) == 0 {
				t.Fatalf("IR порожнє: %d сайтів, %d мереж, %d пристроїв",
					len(m.Sites), len(m.Networks), len(m.Devices))
			}

			for _, s := range m.Sites {
				if s.Gateway == nil {
					t.Errorf("сайт %s без L3-шлюзу, кандидатів %d", s.Name, len(s.GatewayCandidates))
				}
			}
			for _, n := range m.Networks {
				if !n.Prefix.IsValid() {
					t.Errorf("%s: префікс не розібрано", n.Path)
				}
				if n.Site == nil {
					t.Errorf("%s: мережа без сайту", n.Path)
				}
			}
			for _, d := range m.Devices {
				if !d.MgmtIP.IsValid() {
					t.Errorf("%s: mgmt_ip не розібрано", d.Path)
				}
			}
			for _, s := range m.Servers {
				if !s.IP.IsValid() {
					t.Errorf("%s: ip не розібрано", s.Path)
				}
			}

			cn := m.ControlNode
			if cn.Site.Site == nil || cn.Network.Net == nil || !cn.IP.IsValid() {
				t.Errorf("вузол керування не розвʼязано: %+v", cn)
			}
			for _, r := range m.Rules {
				if !r.From.Resolved() || !r.To.Resolved() {
					t.Errorf("%s: правило не розвʼязано: %s → %s", r.Path, r.From.Name, r.To.Name)
				}
			}

			for _, p := range origins(m) {
				if !m.At(p).Known() {
					t.Errorf("шлях %q не має позиції у файлі", p)
				}
			}
		})
	}
}

// origins lists the path of every IR element, in IR order.
func origins(m *ir.Model) []string {
	ps := []string{m.ControlNode.Path}
	for _, s := range m.Sites {
		ps = append(ps, s.Path)
		for _, u := range s.Uplinks {
			ps = append(ps, u.Path)
		}
	}
	for _, n := range m.Networks {
		ps = append(ps, n.Path, n.Path+".cidr")
	}
	for _, d := range m.Devices {
		ps = append(ps, d.Path, d.Path+".mgmt_ip")
	}
	for _, s := range m.Servers {
		ps = append(ps, s.Path, s.Path+".ip")
	}
	for _, r := range m.Rules {
		ps = append(ps, r.Path, r.Path+".from", r.Path+".to")
	}
	return ps
}

// The multi-site profile exercises what a single site cannot: model-wide
// lists in source order, a gateway per site, and references across sites.
func TestMProfileShape(t *testing.T) {
	m := lower(t, load(t, testdata("valid", "m-profile.yml")))

	if m.Profile != ir.ProfileM || !strings.HasSuffix(m.File, "m-profile.yml") {
		t.Errorf("Profile = %q, File = %q", m.Profile, m.File)
	}

	var nets []string
	for _, n := range m.Networks {
		nets = append(nets, n.Site.Name+"/"+n.Name)
	}
	want := "hq/mgmt hq/users hq/servers hq/guest branch/mgmt-branch branch/users-branch branch/servers-branch"
	if got := strings.Join(nets, " "); got != want {
		t.Errorf("мережі в порядку файлу:\n got %s\nwant %s", got, want)
	}

	gw := map[string]string{}
	for _, s := range m.Sites {
		if s.Gateway != nil {
			gw[s.Name] = s.Gateway.Name
		}
	}
	if gw["hq"] != "rb750" || gw["branch"] != "rb-branch" {
		t.Errorf("шлюзи сайтів = %v", gw)
	}

	guest := m.Networks[3]
	if guest.Role != ir.RoleGuest || !guest.Isolated || guest.Pool == nil ||
		guest.Pool.From != netip.MustParseAddr("192.168.40.100") ||
		guest.Pool.To != netip.MustParseAddr("192.168.40.200") {
		t.Errorf("guest = %+v, pool = %+v", guest, guest.Pool)
	}
	if mgmt := m.Networks[0]; mgmt.Pool != nil || mgmt.Gateway != netip.MustParseAddr("192.168.10.1") {
		t.Errorf("mgmt: gateway = %v, pool = %v", mgmt.Gateway, mgmt.Pool)
	}

	cn := m.ControlNode
	if cn.Network.Net != m.Networks[2] || cn.Site.Site != m.Sites[0] {
		t.Errorf("вузол керування вказує не на hq/servers: %+v", cn)
	}

	// servers → mgmt-branch crosses sites: the reference is global.
	r := m.Rules[1]
	if r.From.Net != m.Networks[2] || r.To.Net != m.Networks[4] {
		t.Errorf("правило 1: %s → %s", r.From.Name, r.To.Name)
	}
	if inet := m.Rules[4].To; inet.Kind != ir.EndpointInternet || inet.Net != nil {
		t.Errorf("internet розвʼязано як %+v", inet)
	}
	if !m.Rules[4].Ports.Any {
		t.Error("правило без ports має пропускати будь-який трафік")
	}

	uplinks := m.Sites[1].Uplinks
	if len(uplinks) != 2 || uplinks[1].Kind != ir.UplinkBackup || uplinks[1].Site != m.Sites[1] {
		t.Errorf("канали філії = %+v", uplinks)
	}
}

func TestPortNormalisation(t *testing.T) {
	tests := []struct {
		name  string
		ports []int
		proto string
		want  ir.PortSet
	}{
		{"без портів — будь-який трафік", nil, "", ir.PortSet{Any: true}},
		{"tcp за замовчуванням, відсортовано", []int{8728, 22}, "",
			ir.PortSet{Ports: []ir.PortSpec{{Proto: ir.TCP, Port: 22}, {Proto: ir.TCP, Port: 8728}}}},
		{"явний udp", []int{514}, "udp",
			ir.PortSet{Ports: []ir.PortSpec{{Proto: ir.UDP, Port: 514}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := portSet(tt.ports, tt.proto)
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("portSet(%v, %q) = %+v, want %+v", tt.ports, tt.proto, got, tt.want)
			}
		})
	}

	set := portSet([]int{22, 8728}, "")
	switch {
	case !set.Admits(ir.PortSpec{Proto: ir.TCP, Port: 8728}):
		t.Error("tcp/8728 мав би пройти")
	case set.Admits(ir.PortSpec{Proto: ir.UDP, Port: 8728}):
		t.Error("udp/8728 не мав би пройти: протокол інший")
	case set.Admits(ir.PortSpec{Proto: ir.TCP, Port: 443}):
		t.Error("tcp/443 не мав би пройти")
	}
}

// The step's acceptance criterion: a model whose reference does not resolve
// lowers anyway, and the IR keeps what E003 needs to report it.
func TestUnresolvedRuleReferenceSurvives(t *testing.T) {
	m := lower(t, load(t, testdata("broken", "e003-unknown-rule-segment.yml")))

	if len(m.Rules) != 1 {
		t.Fatalf("правил %d, want 1", len(m.Rules))
	}
	from := m.Rules[0].From
	if from.Kind != ir.EndpointNetwork || from.Name != "srvers" || from.Net != nil || from.Resolved() {
		t.Errorf("from = %+v, want нерозвʼязане посилання на srvers", from)
	}
	if !m.Rules[0].To.Resolved() {
		t.Error("to: mgmt мало розвʼязатися")
	}
	if !m.At(m.Rules[0].Path + ".from").Known() {
		t.Error("для нерозвʼязаного посилання немає позиції — E003 лишиться без рядка")
	}
}

func TestUnresolvedReferencesAndGateways(t *testing.T) {
	minimal := testdata("valid", "minimal.yml")
	mprofile := testdata("valid", "m-profile.yml")

	t.Run("невідомий сайт вузла керування", func(t *testing.T) {
		m := lower(t, loadVariant(t, minimal, "  site: hq\n", "  site: warehouse\n"))
		cn := m.ControlNode
		if cn.Site.Site != nil || cn.Site.Name != "warehouse" {
			t.Errorf("Site = %+v", cn.Site)
		}
		// The network is looked up within the site, so it cannot resolve
		// without one.
		if cn.Network.Net != nil || cn.Network.Name != "servers" {
			t.Errorf("Network = %+v", cn.Network)
		}
	})

	t.Run("мережа вузла керування з іншого сайту", func(t *testing.T) {
		m := lower(t, loadVariant(t, mprofile, "  network: servers\n", "  network: servers-branch\n"))
		if m.ControlNode.Site.Site == nil {
			t.Fatal("сайт hq мав розвʼязатися")
		}
		if m.ControlNode.Network.Net != nil {
			t.Errorf("мережа іншого сайту не може бути мережею вузла керування: %+v", m.ControlNode.Network)
		}
	})

	t.Run("сайт без L3-шлюзу", func(t *testing.T) {
		m := lower(t, loadVariant(t, minimal, "role: edge_router", "role: access_switch"))
		s := m.Sites[0]
		if s.Gateway != nil || len(s.GatewayCandidates) != 0 {
			t.Errorf("Gateway = %v, кандидатів %d", s.Gateway, len(s.GatewayCandidates))
		}
	})

	t.Run("два L3-шлюзи", func(t *testing.T) {
		m := lower(t, loadVariant(t, mprofile,
			"        role: core_switch\n        mgmt_ip: 192.168.110.3\n", "        role: firewall\n        mgmt_ip: 192.168.110.3\n"))
		s := m.Sites[1]
		if s.Gateway != nil || len(s.GatewayCandidates) != 2 {
			t.Errorf("Gateway = %v, кандидатів %d — шлюз має лишитися невизначеним", s.Gateway, len(s.GatewayCandidates))
		}
	})

	t.Run("дублікат імені мережі", func(t *testing.T) {
		// E002 reports the duplicate; until then the reference binds to the
		// first definition, deterministically.
		m := lower(t, loadVariant(t, mprofile, "name: servers-branch", "name: servers"))
		r := m.Rules[0] // servers → mgmt
		if r.From.Net == nil || r.From.Net.Site.Name != "hq" {
			t.Errorf("посилання на дублікат мало вказати на перше визначення (hq): %+v", r.From.Net)
		}
	})
}

// The schema and net/netip must agree on what an address is. If they ever
// diverge, lowering has to say so instead of handing invariants a zero
// address they would silently skip.
func TestUnparsableAddressIsAnOrgcDefect(t *testing.T) {
	doc := load(t, testdata("valid", "minimal.yml"))
	doc.Org.Sites[0].Devices[0].MgmtIP = "192.168.010.1" // what the schema rejects

	_, err := Lower(doc)
	if err == nil {
		t.Fatal("Lower прийняв адресу, яку netip не розбирає")
	}
	if !strings.Contains(err.Error(), "sites[0].devices[0].mgmt_ip") {
		t.Errorf("помилка не називає шлях: %v", err)
	}
}

// Same input, same IR, element for element. dump renders everything the
// invariants can observe, so any order that leaked from a map would show.
func TestLoweringIsDeterministic(t *testing.T) {
	for _, f := range validFiles(t) {
		t.Run(filepath.Base(f), func(t *testing.T) {
			first := dump(lower(t, load(t, f)))
			for i := 0; i < 20; i++ {
				if got := dump(lower(t, load(t, f))); got != first {
					t.Fatalf("прогін %d дав інше IR:\n%s\n---\n%s", i, got, first)
				}
			}
		})
	}
}

func dump(m *ir.Model) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", m.Organization, m.Profile)
	for _, s := range m.Sites {
		gw := "-"
		if s.Gateway != nil {
			gw = s.Gateway.Name
		}
		fmt.Fprintf(&b, "site %s overlap=%v gw=%s cand=%d @%s\n", s.Name, s.AllowOverlap, gw, len(s.GatewayCandidates), s.Path)
		for _, u := range s.Uplinks {
			fmt.Fprintf(&b, "  uplink %s %s\n", u.Name, u.Kind)
		}
	}
	for _, n := range m.Networks {
		fmt.Fprintf(&b, "net %s/%s vlan=%d %s %s gw=%v pool=%v iso=%v @%s\n",
			n.Site.Name, n.Name, n.VLAN, n.Role, n.Prefix, n.Gateway, n.Pool, n.Isolated, n.Path)
	}
	for _, d := range m.Devices {
		fmt.Fprintf(&b, "dev %s %s %s %s %v @%s\n", d.Name, d.Type, d.OSVersion, d.Role, d.MgmtIP, d.Path)
	}
	for _, s := range m.Servers {
		fmt.Fprintf(&b, "srv %s %v %v @%s\n", s.Name, s.Roles, s.IP, s.Path)
	}
	fmt.Fprintf(&b, "cn %s/%s %v\n", m.ControlNode.Site.Name, m.ControlNode.Network.Name, m.ControlNode.IP)
	for _, r := range m.Rules {
		fmt.Fprintf(&b, "rule %d:%s → %d:%s %+v @%s\n", r.From.Kind, r.From.Name, r.To.Kind, r.To.Name, r.Ports, r.Path)
	}
	return b.String()
}
