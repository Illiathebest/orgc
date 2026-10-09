package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"orgc/internal/diag"
	"orgc/schema"
)

// Document is a model that passed the schema, together with what the later
// stages need to point back into the file.
type Document struct {
	// File is the name diagnostics are reported against.
	File string

	Org *Organization

	// Positions locates every value of the document by readable path.
	Positions Positions
}

// Load reads organization.yml, checks it against the embedded schema and
// decodes it.
//
// The three results separate three different things, because the caller has
// to act differently on each:
//
//   - err is a failure to read the file or to build the validator. It means
//     orgc was invoked wrongly or is itself broken, not that the model is.
//   - diagnostics describe the model. They may contain warnings only, in
//     which case the model is still valid.
//   - doc is non-nil exactly when the diagnostics hold no errors.
func Load(path string) (doc *Document, diagnostics []diag.Diagnostic, err error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return LoadBytes(path, src)
}

// LoadBytes is Load for a file already in memory. name is used in
// diagnostics only.
func LoadBytes(name string, src []byte) (*Document, []diag.Diagnostic, error) {
	file, err := parser.ParseBytes(src, 0)
	if err != nil {
		return nil, []diag.Diagnostic{syntaxDiagnostic(name, err)}, nil
	}
	if len(file.Docs) == 0 || file.Docs[0].Body == nil {
		return nil, []diag.Diagnostic{{
			Code:     diag.SYAMLSyntax,
			Severity: diag.Error,
			Message:  "файл порожній: немає жодного документа YAML",
			File:     name,
		}}, nil
	}

	sch, err := validator()
	if err != nil {
		return nil, nil, fmt.Errorf("не вдалося підготувати схему: %w", err)
	}

	var plain any
	if err := yaml.Unmarshal(src, &plain); err != nil {
		return nil, []diag.Diagnostic{syntaxDiagnostic(name, err)}, nil
	}

	loc := newLocator(file.Docs[0].Body)

	if err := sch.Validate(normalize(plain)); err != nil {
		var ve *jsonschema.ValidationError
		if !errors.As(err, &ve) {
			return nil, nil, fmt.Errorf("валідація не виконалася: %w", err)
		}
		ds := collect(ve, name, loc)
		diag.Sort(ds)
		return nil, ds, nil
	}

	var org Organization
	if err := yaml.Unmarshal(src, &org); err != nil {
		// The schema already accepted the document, so a decode failure
		// here means the structures and the schema disagree — a defect in
		// orgc, not in the model.
		return nil, nil, fmt.Errorf("модель пройшла схему, але не розбирається у структури: %w", err)
	}
	return &Document{
		File:      name,
		Org:       &org,
		Positions: indexPositions(file.Docs[0].Body),
	}, nil, nil
}

// goccyPosition matches the "[line:column]" prefix goccy puts on a syntax
// error. The error type carrying the token is internal to that library, so
// the position has to be read back from the text.
var goccyPosition = regexp.MustCompile(`^\[(\d+):(\d+)\]\s*`)

func syntaxDiagnostic(name string, err error) diag.Diagnostic {
	// goccy appends an excerpt of the source after the message; the first
	// line is the message itself.
	text := err.Error()
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}

	d := diag.Diagnostic{
		Code:     diag.SYAMLSyntax,
		Severity: diag.Error,
		File:     name,
	}
	if m := goccyPosition.FindStringSubmatch(text); m != nil {
		d.Line, _ = strconv.Atoi(m[1])
		d.Column, _ = strconv.Atoi(m[2])
		text = goccyPosition.ReplaceAllString(text, "")
	}
	d.Message = "файл не є коректним YAML: " + strings.TrimSpace(text)
	return d
}

var (
	validatorOnce sync.Once
	compiled      *jsonschema.Schema
	compileErr    error
)

// validator compiles the embedded schema once. Compilation is pure, so the
// result is safe to share.
func validator() (*jsonschema.Schema, error) {
	validatorOnce.Do(func() {
		const url = "organization.schema.json"

		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.Organization))
		if err != nil {
			compileErr = fmt.Errorf("вбудована схема не є коректним JSON: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(url, doc); err != nil {
			compileErr = err
			return
		}
		compiled, compileErr = c.Compile(url)
	})
	return compiled, compileErr
}

// normalize converts a YAML-decoded value into the shape the validator
// expects: every number as json.Number, every mapping keyed by string.
//
// YAML allows non-string keys; JSON Schema does not. Such a key cannot be
// described by the schema at all, so it is rendered to its string form and
// then reported as an unknown field — which is what it is.
func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[k] = normalize(val)
		}
		return m
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[fmt.Sprintf("%v", k)] = normalize(val)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, val := range t {
			s[i] = normalize(val)
		}
		return s
	case int:
		return json.Number(strconv.Itoa(t))
	case int64:
		return json.Number(strconv.FormatInt(t, 10))
	case uint64:
		return json.Number(strconv.FormatUint(t, 10))
	case float64:
		return json.Number(strconv.FormatFloat(t, 'f', -1, 64))
	default:
		return v
	}
}
