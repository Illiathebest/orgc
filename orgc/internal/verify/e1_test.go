package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"orgc/internal/lower"
	"orgc/internal/model"
)

// TestE1Timings times each stage of verification on the models of the
// scaling experiment: schema and parsing, lowering, then every invariant on
// its own, in registry order (docs/adr/ADR-008, decision 3). It is skipped
// unless ORGC_E1_MODELS names a directory of models; the result goes to
// ORGC_E1_OUT as JSON. ORGC_E1_RUNS sets how many times each stage is
// benchmarked (default 11); the first one warms up and is not counted.
//
//	ORGC_E1_MODELS=dir ORGC_E1_OUT=timings.json go test -run TestE1Timings ./internal/verify
func TestE1Timings(t *testing.T) {
	dir := os.Getenv("ORGC_E1_MODELS")
	if dir == "" {
		t.Skip("ORGC_E1_MODELS не задано: це замір E1, а не тест")
	}
	runs := 11
	if v := os.Getenv("ORGC_E1_RUNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2 {
			t.Fatalf("ORGC_E1_RUNS=%q", v)
		}
		runs = n
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("моделей у %s немає: %v", dir, err)
	}
	sort.Strings(files)

	// Each stage is timed with testing.Benchmark, which repeats it until the
	// total is long enough for the clock — so that a check taking a few
	// microseconds is measured, not rounded to the timer's resolution. The
	// benchmark itself is repeated: median and spread of ns/op over the
	// repetitions, the first one dropped as a warm-up.
	measure := func(f func()) map[string]float64 {
		var d []float64
		for i := 0; i < runs; i++ {
			r := testing.Benchmark(func(b *testing.B) {
				for n := 0; n < b.N; n++ {
					f()
				}
			})
			if i > 0 {
				d = append(d, float64(r.NsPerOp())/1e3)
			}
		}
		sort.Float64s(d)
		q := func(p float64) float64 { return d[int(p*float64(len(d)-1)+0.5)] }
		return map[string]float64{"median_us": q(0.5), "q1_us": q(0.25), "q3_us": q(0.75), "min_us": d[0], "max_us": d[len(d)-1]}
	}

	ps := platforms(t)
	out := map[string]any{"runs": runs - 1}
	results := map[string]any{}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		doc, ds, err := model.LoadBytes(name, src)
		if err != nil || doc == nil {
			t.Fatalf("%s не пройшов схему: %v %v", name, err, ds)
		}
		m, err := lower.Lower(doc)
		if err != nil {
			t.Fatal(err)
		}
		r := map[string]any{
			"devices":  len(m.Devices),
			"networks": len(m.Networks),
			"rules":    len(m.Rules),
			"sites":    len(m.Sites),
			"load":     measure(func() { model.LoadBytes(name, src) }),
			"lower":    measure(func() { lower.Lower(doc) }),
		}
		checks := map[string]any{}
		for _, c := range Checks() {
			checks[string(c.Code)] = measure(func() { c.Run(m, ps) })
		}
		r["checks"] = checks
		r["verify_all"] = measure(func() { Run(m, ps, Checks()) })
		results[name] = r
		t.Logf("%s: %d пристроїв", name, len(m.Devices))
	}
	out["models"] = results
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if p := os.Getenv("ORGC_E1_OUT"); p != "" {
		if err := os.WriteFile(p, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
