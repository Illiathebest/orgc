package verify

import (
	"strings"
	"testing"
)

const (
	mStandbyAcc01 = "      - { a: dist-02:Ethernet3, b: acc-01:Ethernet2, vlans: [mgmt, servers], standby: true }\n"
	mStandbyAcc03 = "      - { a: dist-01:Ethernet4, b: acc-03:Ethernet2, vlans: [mgmt, users, guest], standby: true }\n"
	mActiveAcc01  = "      - { a: dist-01:Ethernet2, b: acc-01:Ethernet1, vlans: [mgmt, servers] }\n"
	mActiveAcc03  = "      - { a: dist-02:Ethernet2, b: acc-03:Ethernet1, vlans: [mgmt, users, guest] }\n"
)

// Standby links: what the tree and STP accept, W006 for a failover path
// without the mgmt network, E010 for a standby link that closes no cycle or
// duplicates another. Every check is registered, so each case also proves
// no other code fires.
func TestStandbyLinks(t *testing.T) {
	tests := []struct {
		name  string
		edits []string
		want  string
		msg   []string // fragments of the messages, in order
	}{
		{"резервні зʼєднання m-profile — без діагностик", nil, "", nil},
		{"резервне паралельно активному: другий кабель, який блокує STP",
			[]string{mStandbyAcc01, mStandbyAcc01 + "      - { a: dist-01:Ethernet5, b: acc-01:Ethernet3, vlans: [mgmt], standby: true }\n"}, "", nil},
		{"резервне без mgmt-мережі — W006",
			[]string{mStandbyAcc01, strings.Replace(mStandbyAcc01, "[mgmt, servers]", "[servers]", 1)}, "W006",
			[]string{`резервне зʼєднання dist-02:Ethernet3 – acc-01:Ethernet2 не пропускає mgmt-мережу "mgmt", тож пристрої можуть втратити керування: ` +
				`при відмові зʼєднання core-01:Ethernet2 – dist-01:Ethernet1 — "dist-01", "acc-01", "acc-02"; ` +
				`при відмові зʼєднання core-01:Ethernet3 – dist-02:Ethernet1 — "dist-02", "acc-03"`}},
		{"резервне без vlans несе всі мережі",
			[]string{mStandbyAcc01, strings.Replace(mStandbyAcc01, "vlans: [mgmt, servers], ", "", 1)}, "", nil},
		// acc-01 gets no mgmt even now: E006 reports the link, and the
		// failover paths of the others, which cross that same link, are the
		// same defect — no W006 on top (one defect, one code).
		{"резервний шлях іде вгору через зʼєднання без mgmt — лише E006",
			[]string{mActiveAcc01, strings.Replace(mActiveAcc01, "[mgmt, servers]", "[servers]", 1)}, "E006",
			[]string{`"acc-01" недосяжний`}},
		{"два резервні між тією самою парою — E010",
			[]string{mStandbyAcc01, mStandbyAcc01 + "      - { a: dist-02:Ethernet5, b: acc-01:Ethernet3, standby: true }\n"}, "E010",
			[]string{"друге резервне між пристроями"}},
		{"резервне до пристрою без активного шляху — E010",
			[]string{mStandbyAcc03, "", mActiveAcc03, strings.Replace(mActiveAcc03, "] }", "], standby: true }", 1)}, "E010",
			[]string{`пристрій "acc-03" не має активного шляху до шлюзу "rb750"`}},
		{"резервне зʼєднання пристрою із самим собою — E010",
			[]string{mStandbyAcc01, mStandbyAcc01 + "      - { a: acc-01:Ethernet3, b: acc-01:Ethernet4, standby: true }\n"}, "E010",
			[]string{"сам із собою"}},
		{"цикл з активних зʼєднань — E010 з підказкою про standby",
			[]string{mStandbyAcc01, strings.Replace(mStandbyAcc01, ", standby: true }", " }", 1)}, "E010",
			[]string{"резервне позначають standby: true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := runChecks(t, variant(t, mprofile(), tt.edits...), Checks())
			if codes(ds) != tt.want {
				t.Fatalf("коди %q, want %q:\n%s", codes(ds), tt.want, render(ds))
			}
			for i, frag := range tt.msg {
				if !strings.Contains(ds[i].Message, frag) {
					t.Errorf("повідомлення %d без %q: %s", i, frag, ds[i])
				}
			}
		})
	}
}

// Without standby links W006 has nothing to check, whatever the vlans.
func TestW006NeedsStandbyLinks(t *testing.T) {
	ds := runChecks(t, variant(t, sprofile()), []Check{{Code: "W006", Run: checkW006}})
	if len(ds) != 0 {
		t.Fatalf("W006 без резервних зʼєднань:\n%s", render(ds))
	}
}
