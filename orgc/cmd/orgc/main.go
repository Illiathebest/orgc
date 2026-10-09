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

Коди виходу:
  0    модель коректна
  1    у моделі знайдено помилки
  2    помилка виклику: невідома команда, немає файлу, файл не читається

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

	// Structural validation lands in internal/model (plan step 13); the
	// invariants follow in internal/verify. Until then the command must not
	// report success it has not established.
	fmt.Fprintf(stderr, "orgc: перевірку моделі ще не реалізовано (%s читається, але не аналізується)\n", path)
	return exitInvoked
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

	fmt.Fprintf(stderr, "orgc: генерацію ще не реалізовано (%s → %s)\n", path, *outDir)
	return exitInvoked
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
