package verify

import (
	"strings"
	"testing"

	"orgc/internal/diag"
)

var (
	e002 = []Check{{Code: diag.E002, Run: checkE002}}
	e003 = []Check{{Code: diag.E003, Run: checkE003}}
)

// Scopes are what E002 is about: an identifier may repeat wherever its
// scope allows it.
func TestE002ScopesAllowReuse(t *testing.T) {
	mprofile := testdata("valid", "m-profile.yml")

	t.Run("однаковий VLAN у різних сайтах", func(t *testing.T) {
		m := variant(t, mprofile, "vlan: 110", "vlan: 10")
		if ds := runChecks(t, m, e002); len(ds) != 0 {
			t.Errorf("VLAN унікальний лише в сайті, а отримано:\n%s", render(ds))
		}
	})
	t.Run("однакове імʼя каналу в різних сайтах", func(t *testing.T) {
		m := variant(t, mprofile, "name: lte-branch", "name: lte-backup")
		if ds := runChecks(t, m, e002); len(ds) != 0 {
			t.Errorf("імʼя каналу унікальне лише в сайті, а отримано:\n%s", render(ds))
		}
	})
}

func TestE002ModelWideScopes(t *testing.T) {
	mprofile := testdata("valid", "m-profile.yml")

	t.Run("мережа з тим самим імʼям в іншому сайті", func(t *testing.T) {
		m := variant(t, mprofile, "name: servers-branch", "name: servers")
		ds := runChecks(t, m, e002)
		if len(ds) != 1 || !strings.Contains(ds[0].Message, `уже є в сайті "hq"`) {
			t.Errorf("імена мереж унікальні в моделі:\n%s", render(ds))
		}
	})
	t.Run("пристрій з імʼям пристрою іншого сайту", func(t *testing.T) {
		m := variant(t, mprofile, "name: rb-branch", "name: rb750")
		if ds := runChecks(t, m, e002); len(ds) != 1 || ds[0].Path != "sites[1].devices[0].name" {
			t.Errorf("імена пристроїв унікальні в моделі:\n%s", render(ds))
		}
	})
	t.Run("триразове імʼя дає дві діагностики", func(t *testing.T) {
		m := variant(t, mprofile,
			"name: mon-01,", "name: ctrl-01,",
			"name: app-branch-01,", "name: ctrl-01,")
		if ds := runChecks(t, m, e002); len(ds) != 2 {
			t.Errorf("перше визначення не є дублікатом, решта — є:\n%s", render(ds))
		}
	})
}

func TestE003ResolvedReferencesPass(t *testing.T) {
	mprofile := testdata("valid", "m-profile.yml")

	t.Run("m-profile: правила між сайтами, internet, шлюз у кожному сайті", func(t *testing.T) {
		if ds := runChecks(t, lowerFile(t, mprofile), e003); len(ds) != 0 {
			t.Errorf("усі посилання розвʼязуються, а отримано:\n%s", render(ds))
		}
	})
	t.Run("any як кінець правила", func(t *testing.T) {
		m := variant(t, mprofile, "{ from: users, to: internet,", "{ from: users, to: any,")
		if ds := runChecks(t, m, e003); len(ds) != 0 {
			t.Errorf("any — коректне посилання:\n%s", render(ds))
		}
	})
	t.Run("сайт без мереж зі шлюзом не потребує L3-шлюзу", func(t *testing.T) {
		m := variant(t, testdata("valid", "minimal.yml"),
			"cidr: 192.168.10.0/24, gateway: 192.168.10.1 }", "cidr: 192.168.10.0/24 }",
			"cidr: 192.168.30.0/24, gateway: 192.168.30.1 }", "cidr: 192.168.30.0/24 }",
			"role: edge_router", "role: access_switch")
		if ds := runChecks(t, m, e003); len(ds) != 0 {
			t.Errorf("L3-шлюз потрібен лише мережам з gateway:\n%s", render(ds))
		}
	})
}

// Both ends of a rule are checked, each with its own position.
func TestE003ReportsBothRuleEnds(t *testing.T) {
	m := variant(t, testdata("valid", "minimal.yml"),
		"{ from: servers, to: mgmt,", "{ from: srvers, to: mgnt,")
	ds := runChecks(t, m, e003)
	if len(ds) != 2 {
		t.Fatalf("діагностик %d, want 2:\n%s", len(ds), render(ds))
	}
	if !strings.HasSuffix(ds[0].Path, ".from") || !strings.HasSuffix(ds[1].Path, ".to") {
		t.Errorf("шляхи: %s, %s", ds[0].Path, ds[1].Path)
	}
	if ds[0].Line != ds[1].Line || ds[0].Column >= ds[1].Column {
		t.Errorf("from і to на одному рядку мають різні колонки: %d:%d, %d:%d",
			ds[0].Line, ds[0].Column, ds[1].Line, ds[1].Column)
	}
}
