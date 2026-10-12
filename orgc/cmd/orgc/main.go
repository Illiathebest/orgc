// Command orgc compiles a declarative model of an organization into network
// device configurations, verifying security invariants before anything is
// applied. It never applies configuration itself — it only produces text.
//
// User-facing text is Ukrainian: the administrator is the audience.
// Identifiers and comments stay English.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"orgc/internal/backend"
	"orgc/internal/backend/ansible"
	"orgc/internal/backend/aristaeos"
	"orgc/internal/backend/containerlab"
	"orgc/internal/backend/prometheus"
	"orgc/internal/backend/routeros"
	"orgc/internal/diag"
	"orgc/internal/ir"
	"orgc/internal/lower"
	"orgc/internal/model"
	"orgc/internal/render"
	"orgc/internal/verify"
)

// Exit codes form part of the CLI contract: CI distinguishes "the model is
// wrong" from "orgc was called wrong", and must never confuse the two.
const (
	exitOK      = 0 // model is valid; nothing to report
	exitModel   = 1 // the model contains errors
	exitInvoked = 2 // bad invocation, unreadable file, or unimplemented path
)

const usage = `orgc — компілятор декларативної моделі організації в конфігурації
мережевої інфраструктури з перевіркою інваріантів безпеки.

Використання:
  orgc <команда> <organization.yml>

Команди:
  verify    перевірити модель; друкує діагностики або OK
  render    згенерувати конфігурації пристроїв та інвентарі

Прапорці:
  -h, --help    показати цю довідку
  -o <каталог>  (render) каталог виходу, типово out; перезаписується
                повністю, якщо створений orgc render

Коди виходу:
  0    модель коректна; попередження код виходу не змінюють
  1    у моделі знайдено помилки (зокрема W002 рівня помилки);
       render у такому разі не створює жодного файлу
  2    помилка виклику: невідома команда, немає файлу, файл не читається,
       каталог виходу не порожній і створений не orgc

orgc нічого не застосовує — лише генерує текст. Застосування виконує
наявний ланцюг GitLab CI → Ansible.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run holds the whole CLI so that tests can drive it without touching
// os.Args or the real streams.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitInvoked
	}

	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return exitOK
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "render":
		return runRender(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "orgc: невідома команда %q\n\n", args[0])
		fmt.Fprint(stderr, usage)
		return exitInvoked
	}
}

func runVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return exitInvoked
	}

	path, code := singleModelArg(fs.Args(), "verify", stderr)
	if code != exitOK {
		return code
	}
	if code := checkReadable(path, stderr); code != exitOK {
		return code
	}

	ds, err := verifyModel(path)
	if err != nil {
		fmt.Fprintf(stderr, "orgc: %v\n", err)
		return exitInvoked
	}

	// Findings go to stdout: they are the output of the command, not a
	// complaint about how it was called. Render sorts them itself.
	if err := diag.Render(stdout, ds); err != nil {
		fmt.Fprintf(stderr, "orgc: не вдалося вивести діагностики: %v\n", err)
		return exitInvoked
	}
	fmt.Fprintln(stdout, diag.Summary(ds))

	// Warnings alone leave the model valid; an error of any code fails it,
	// W002 raised to error level included (docs/adr/ADR-008, decision 4).
	if diag.HasErrors(ds) {
		return exitModel
	}
	return exitOK
}

// invariants is the registry verify runs. A variable so that tests can put
// checks with known findings in it; the program itself never reassigns it.
var invariants = verify.Checks

// platforms is the set of platforms orgc is built with. It is assembled here,
// outside the core, so that the core never imports a vendor package.
func platforms() (*backend.Registry, error) {
	return backend.NewRegistry(routeros.Platform{}, aristaeos.Platform{})
}

// generators is the registry of model-wide generators render runs, in
// order. A variable for the same reason as invariants; backends join it as
// they are implemented.
var generators = func() []backend.Generator {
	return []backend.Generator{ansible.Generator{}, containerlab.Generator{}, prometheus.Generator{}}
}

// verifyModel runs the pipeline up to verification and returns its findings.
func verifyModel(path string) ([]diag.Diagnostic, error) {
	_, ds, err := pipeline(path)
	return ds, err
}

// pipeline runs schema, lowering and invariants — the one path both verify
// and render take, so they can never judge a model differently. A model
// with structural errors stops after the schema: it has no definite meaning
// for the invariants to check, and the returned IR is nil.
//
// The error means orgc itself failed, never that the model is wrong.
func pipeline(path string) (*ir.Model, []diag.Diagnostic, error) {
	doc, ds, err := model.Load(path)
	if err != nil || doc == nil {
		return nil, ds, err
	}

	m, err := lower.Lower(doc)
	if err != nil {
		return nil, nil, err
	}
	reg, err := platforms()
	if err != nil {
		return nil, nil, err
	}
	found, err := verify.Run(m, reg, invariants())
	if err != nil {
		return nil, nil, err
	}
	return m, append(ds, found...), nil
}

func runRender(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	outDir := fs.String("o", "out", "каталог для згенерованих файлів")
	if err := fs.Parse(args); err != nil {
		return exitInvoked
	}

	path, code := singleModelArg(fs.Args(), "render", stderr)
	if code != exitOK {
		return code
	}
	if code := checkReadable(path, stderr); code != exitOK {
		return code
	}

	m, ds, err := pipeline(path)
	if err != nil {
		fmt.Fprintf(stderr, "orgc: %v\n", err)
		return exitInvoked
	}

	// Generation follows verification, and only a clean one: a model with
	// errors gets its diagnostics and no files at all, and the output
	// directory is left untouched (docs/adr/ADR-009, decision 1).
	if err := diag.Render(stdout, ds); err != nil {
		fmt.Fprintf(stderr, "orgc: не вдалося вивести діагностики: %v\n", err)
		return exitInvoked
	}
	if m == nil || diag.HasErrors(ds) {
		fmt.Fprintln(stdout, diag.Summary(ds))
		return exitModel
	}

	reg, err := platforms()
	if err != nil {
		fmt.Fprintf(stderr, "orgc: %v\n", err)
		return exitInvoked
	}
	files, err := render.Files(m, reg, generators())
	if err != nil {
		fmt.Fprintf(stderr, "orgc: %v\n", err)
		return exitInvoked
	}
	if err := render.Write(*outDir, files); err != nil {
		if errors.Is(err, render.ErrNotOwned) {
			fmt.Fprintf(stderr, "orgc: %v; вкажіть порожній каталог або каталог, створений orgc render\n", err)
		} else {
			fmt.Fprintf(stderr, "orgc: не вдалося записати %s: %v\n", *outDir, err)
		}
		return exitInvoked
	}
	if len(ds) > 0 {
		fmt.Fprintln(stdout, diag.Summary(ds))
	}
	fmt.Fprintf(stdout, "Згенеровано файлів: %d → %s\n", len(files), *outDir)
	return exitOK
}

// singleModelArg enforces that a command got exactly one model file.
func singleModelArg(args []string, cmd string, stderr io.Writer) (string, int) {
	switch len(args) {
	case 1:
		return args[0], exitOK
	case 0:
		fmt.Fprintf(stderr, "orgc %s: не вказано файл моделі\n\n", cmd)
	default:
		fmt.Fprintf(stderr, "orgc %s: очікується рівно один файл моделі, отримано %d\n\n", cmd, len(args))
	}
	fmt.Fprint(stderr, usage)
	return "", exitInvoked
}

// checkReadable separates "the file is not there" from "the model is wrong":
// the first is the caller's mistake, the second is the point of the tool.
func checkReadable(path string, stderr io.Writer) int {
	f, err := os.Open(path)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			fmt.Fprintf(stderr, "orgc: файл не знайдено: %s\n", path)
		case errors.Is(err, os.ErrPermission):
			fmt.Fprintf(stderr, "orgc: немає доступу до файлу: %s\n", path)
		default:
			fmt.Fprintf(stderr, "orgc: не вдалося відкрити %s: %v\n", path, err)
		}
		return exitInvoked
	}
	defer f.Close()

	if info, err := f.Stat(); err == nil && info.IsDir() {
		fmt.Fprintf(stderr, "orgc: це каталог, а не файл моделі: %s\n", path)
		return exitInvoked
	}
	return exitOK
}
