package audit

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/iderex/kanzlei/internal/authz"
)

// update regenerates docs/audit.md from the declaration instead of comparing
// against it. The document is generated rather than written, and this flag is
// how it is generated:
//
//	go test ./internal/audit -run TestTheDocumentIsGeneratedFromTheDeclaration -update
var update = flag.Bool("update", false, "rewrite docs/audit.md from the declaration in record.go")

const documentPath = "../../docs/audit.md"

func aRecord() Record {
	return Record{
		Version:       Version,
		ID:            "01J000000000000000000000AA",
		At:            time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC),
		Class:         ClassAuthorisation,
		Principal:     Principal{Subject: "sub-alice", Session: "sess-1"},
		SourceAddress: "198.51.100.7",
		Object:        Object{Source: "file-server", ID: "doc-4711"},
		Outcome:       OutcomeRefused,
		Reason:        ReasonUnrecognisedTerm,
		Detail:        map[DetailKey]string{DetailTerm: "device-posture:unmanaged"},
		Correlation:   "req-88",
	}
}

func TestAWellFormedRecordIsAccepted(t *testing.T) {
	if err := aRecord().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// TestARecordThatCouldNotBeQueriedIsRefused is the table. Each case is a
// record that would sit in the trail looking like evidence and answer no
// question.
func TestARecordThatCouldNotBeQueriedIsRefused(t *testing.T) {
	cases := map[string]func(*Record){
		"no identifier":              func(r *Record) { r.ID = "" },
		"no time":                    func(r *Record) { r.At = time.Time{} },
		"no correlation identifier":  func(r *Record) { r.Correlation = "" },
		"an undeclared event class":  func(r *Record) { r.Class = "retrieval" },
		"an undeclared outcome":      func(r *Record) { r.Outcome = "partially-allowed" },
		"an undeclared reason":       func(r *Record) { r.Reason = "seemed-fine" },
		"an undeclared detail key":   func(r *Record) { r.Detail = map[DetailKey]string{"passage": "x"} },
		"an undeclared layer":        func(r *Record) { r.Detail = map[DetailKey]string{DetailLayer: "somewhere"} },
		"a decision with no outcome": func(r *Record) { r.Outcome = "" },
		"a decision with no reason":  func(r *Record) { r.Reason = "" },
		"a detail value with a paragraph in it": func(r *Record) {
			r.Detail = map[DetailKey]string{DetailTerm: strings.Repeat("x", maxDetailValue+1)}
		},
	}

	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			record := aRecord()
			damage(&record)
			if err := record.Validate(); err == nil {
				t.Fatal("the record was accepted")
			}
		})
	}
}

// TestARecordFromAVersionThisCodeDoesNotKnowIsRefused covers the replay in
// #45. Reading the fields this code recognises and assuming the rest were
// absent turns a record written by a later version into evidence of something
// that did not happen.
func TestARecordFromAVersionThisCodeDoesNotKnowIsRefused(t *testing.T) {
	for _, version := range []int{0, Version - 1, Version + 1} {
		record := aRecord()
		record.Version = version
		if err := record.Validate(); err == nil {
			t.Fatalf("a record at version %d was accepted", version)
		}
	}
}

// TestTheAuthorisationReasonsMatchTheEvaluator holds the two vocabularies
// together.
//
// internal/authz refuses with its own reason codes and this package declares
// the ones a record may carry. Two lists of the same strings drift the first
// time somebody adds a refusal to the evaluator, and the drift shows up as an
// authorisation record that cannot be written for the one case that mattered.
func TestTheAuthorisationReasonsMatchTheEvaluator(t *testing.T) {
	fromEvaluator := []authz.Reason{
		authz.ReasonEmptySet, authz.ReasonUnrecognisedTerm, authz.ReasonUnrecognisedEffect,
		authz.ReasonDeniedByEntry, authz.ReasonMatchedNothing, authz.ReasonAllowedByEntry,
	}

	for _, reason := range fromEvaluator {
		if !slices.Contains(Reasons, Reason(reason)) {
			t.Errorf("the evaluator refuses with %q and no audit reason carries it", reason)
		}
	}

	// The other direction, for the six this test names. A reason declared here
	// that the evaluator does not have is legitimate, because a sign-on and an
	// unreachable authority are refusals the evaluator never takes, so only the
	// six are compared and the rest are listed in docs/audit.md.
	declaredByBoth := 0
	for _, reason := range Reasons {
		if slices.Contains(fromEvaluator, authz.Reason(reason)) {
			declaredByBoth++
		}
	}
	if declaredByBoth != len(fromEvaluator) {
		t.Errorf("%d of the evaluator's %d reasons are declared here", declaredByBoth, len(fromEvaluator))
	}
}

// identifierFields are the three fields of Record that hold a bare string, and
// the reason each one may.
//
// All three carry an identifier minted outside this package: a record
// identifier, a network address and a correlation identifier. Giving each a
// named type here would not constrain what a caller puts in it, and it would
// suggest this package validates a shape it does not.
var identifierFields = map[string]string{
	"ID":            "an identifier minted by whoever wrote the record",
	"SourceAddress": "a network address as the transport reported it",
	"Correlation":   "an identifier minted at the edge of the request",
}

// TestNoFieldTakesFreeText holds the fourth done-when line at the declaration
// rather than at the call site.
//
// A field typed as a bare string is a field somebody writes a sentence into,
// and the sentence is where a passage from the document ends up. Every field
// of Record is a declared type, apart from the three above, and a new bare
// string field is refused here rather than in review.
func TestNoFieldTakesFreeText(t *testing.T) {
	declared := declaredTypes(t)

	for _, field := range recordFields(t) {
		if _, allowed := identifierFields[field.name]; allowed {
			continue
		}
		switch field.typeName {
		case "int", "time.Time", "map[DetailKey]string":
			continue
		}
		if !slices.Contains(declared, field.typeName) {
			t.Errorf("Record.%s is a %s; a field that is not a declared type of this package is a field somebody writes a sentence into",
				field.name, field.typeName)
		}
	}
}

// contentFree is every detail key whose declared meaning is an identifier or a
// code, checked one by one by whoever added it.
//
// It is a second list on purpose. A key added to DetailKeys and not to this one
// fails, which is a person being asked whether the thing that key names can
// carry a passage out of a document.
var contentFree = []DetailKey{DetailTerm, DetailSource, DetailClaim, DetailCount, DetailLayer}

// TestNoDetailKeyCarriesContent is the other half of the fourth done-when
// line. The detail is structured, and every structure it may hold has been
// looked at.
func TestNoDetailKeyCarriesContent(t *testing.T) {
	for _, key := range DetailKeys {
		if !slices.Contains(contentFree, key) {
			t.Errorf("detail key %q is declared and has not been checked for carrying document content; add it to contentFree only after deciding it cannot", key)
		}
	}
	for _, key := range contentFree {
		if !slices.Contains(DetailKeys, key) {
			t.Errorf("contentFree names %q and DetailKeys does not declare it", key)
		}
	}
}

// TestEveryDeclaredSetMatchesItsConstants refuses the list and the constants
// drifting apart, which is how a class becomes undeclared while still being
// emitted.
func TestEveryDeclaredSetMatchesItsConstants(t *testing.T) {
	constants := declaredConstants(t)

	for name, declared := range map[string][]string{
		"Class":     asText(Classes),
		"Reason":    asText(Reasons),
		"Outcome":   asText(Outcomes),
		"DetailKey": asText(DetailKeys),
		"Layer":     asText(Layers),
	} {
		found := constants[name]
		slices.Sort(found)
		sorted := slices.Clone(declared)
		slices.Sort(sorted)
		if !slices.Equal(found, sorted) {
			t.Errorf("%s: constants declare %v and the exported set holds %v", name, found, sorted)
		}
	}
}

// TestTheDocumentIsGeneratedFromTheDeclaration holds the sixth done-when line.
//
// docs/audit.md is produced from record.go, so a field or a class added
// without regenerating it reds this test rather than leaving a document that
// describes the record shape of some earlier week.
func TestTheDocumentIsGeneratedFromTheDeclaration(t *testing.T) {
	generated := document(t)

	if *update {
		if err := os.WriteFile(documentPath, []byte(generated), 0o644); err != nil {
			t.Fatalf("write %s: %v", documentPath, err)
		}
		t.Log("docs/audit.md rewritten from the declaration")
		return
	}

	onDisk, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatalf("read %s: %v", documentPath, err)
	}
	if string(onDisk) != generated {
		t.Fatalf("docs/audit.md does not match the declaration in record.go. Regenerate it:\n\n    go test ./internal/audit -run TestTheDocumentIsGeneratedFromTheDeclaration -update\n")
	}
}

// --- reading the declaration ---

type field struct {
	name     string
	typeName string
	doc      string
}

func parseRecordSource(t *testing.T) (*token.FileSet, *ast.File) {
	t.Helper()
	src, err := os.ReadFile("record.go")
	if err != nil {
		t.Fatalf("read record.go: %v", err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "record.go", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse record.go: %v", err)
	}
	return fset, file
}

// structFields is the fields of one struct declared in record.go, with the
// first sentence of each field's comment.
func structFields(t *testing.T, name string) []field {
	t.Helper()
	_, file := parseRecordSource(t)

	var fields []field
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok || spec.Name.Name != name {
			return true
		}
		structure, ok := spec.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, declared := range structure.Fields.List {
			for _, ident := range declared.Names {
				fields = append(fields, field{
					name:     ident.Name,
					typeName: renderType(declared.Type),
					doc:      firstSentence(declared.Doc.Text(), ident.Name),
				})
			}
		}
		return false
	})
	return fields
}

func recordFields(t *testing.T) []field { return structFields(t, "Record") }

// declaredTypes is every type record.go declares.
func declaredTypes(t *testing.T) []string {
	t.Helper()
	_, file := parseRecordSource(t)

	var names []string
	ast.Inspect(file, func(node ast.Node) bool {
		if spec, ok := node.(*ast.TypeSpec); ok {
			names = append(names, spec.Name.Name)
		}
		return true
	})
	return names
}

// declaredConstants is the value of every exported constant in record.go,
// grouped by the type it was declared with.
func declaredConstants(t *testing.T) map[string][]string {
	t.Helper()
	_, file := parseRecordSource(t)

	byType := map[string][]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || spec.Type == nil || len(spec.Values) != 1 {
			return true
		}
		literal, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		typeName := renderType(spec.Type)
		byType[typeName] = append(byType[typeName], strings.Trim(literal.Value, "`\""))
		return true
	})
	return byType
}

// constantDocs is the value and the first sentence of the comment for every
// string constant of one type.
func constantDocs(t *testing.T, typeName string) [][2]string {
	t.Helper()
	_, file := parseRecordSource(t)

	var out [][2]string
	for _, decl := range file.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || value.Type == nil || renderType(value.Type) != typeName || len(value.Values) != 1 {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok {
				continue
			}
			out = append(out, [2]string{
				strings.Trim(literal.Value, "`\""),
				firstSentence(value.Doc.Text(), value.Names[0].Name),
			})
		}
	}
	return out
}

func renderType(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return renderType(typed.X) + "." + typed.Sel.Name
	case *ast.MapType:
		return "map[" + renderType(typed.Key) + "]" + renderType(typed.Value)
	case *ast.ArrayType:
		return "[]" + renderType(typed.Elt)
	default:
		return ""
	}
}

// firstSentence is a doc comment reduced to one sentence on one line, which is
// what a table cell holds.
func firstSentence(doc, name string) string {
	doc = strings.TrimSpace(strings.ReplaceAll(doc, "\n", " "))
	for strings.Contains(doc, "  ") {
		doc = strings.ReplaceAll(doc, "  ", " ")
	}
	if cut := strings.Index(doc, ". "); cut >= 0 {
		doc = doc[:cut+1]
	}
	doc = strings.TrimSuffix(doc, ".")
	// Drop the Go convention of opening with the identifier, so the cell reads
	// as a description rather than as a repetition of the column beside it.
	for _, opener := range []string{"A " + name + " ", "An " + name + " ", "The " + name + " ", name + " "} {
		if rest, cut := strings.CutPrefix(doc, opener); cut {
			doc = strings.TrimPrefix(strings.TrimPrefix(rest, "is "), "are ")
			break
		}
	}
	if doc == "" {
		return "(undocumented)"
	}
	return strings.ToUpper(doc[:1]) + doc[1:]
}

func asText[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

// document renders docs/audit.md from the declaration in record.go.
func document(t *testing.T) string {
	t.Helper()

	var b strings.Builder
	write := func(format string, args ...any) {
		fmt.Fprintf(&b, format, args...)
	}

	write("# The audit record\n\n")
	write("Generated from the declaration in `internal/audit/record.go`. Do not edit\n")
	write("this file by hand; change the declaration and regenerate it:\n\n")
	write("    go test ./internal/audit -run TestTheDocumentIsGeneratedFromTheDeclaration -update\n\n")
	write("`TestTheDocumentIsGeneratedFromTheDeclaration` compares the two, so a\n")
	write("declaration that changed without this file being regenerated reds the suite.\n\n")
	write("Schema version %d.\n\n", Version)

	write("## Fields\n\n")
	write("| Field | Type | Meaning |\n| --- | --- | --- |\n")
	for _, f := range recordFields(t) {
		write("| `%s` | `%s` | %s |\n", f.name, f.typeName, f.doc)
	}
	write("\n")

	for _, nested := range []string{"Principal", "Object"} {
		write("### `%s`\n\n", nested)
		write("| Field | Type | Meaning |\n| --- | --- | --- |\n")
		for _, f := range structFields(t, nested) {
			write("| `%s` | `%s` | %s |\n", f.name, f.typeName, f.doc)
		}
		write("\n")
	}

	for _, set := range []struct{ heading, typeName string }{
		{"Event classes", "Class"},
		{"Outcomes", "Outcome"},
		{"Reasons", "Reason"},
		{"Detail keys", "DetailKey"},
		{"Layers", "Layer"},
	} {
		write("## %s\n\n", set.heading)
		write("| Value | Meaning |\n| --- | --- |\n")
		for _, entry := range constantDocs(t, set.typeName) {
			write("| `%s` | %s |\n", entry[0], entry[1])
		}
		write("\n")
	}

	write("## What this document does not say\n\n")
	write("Where records are written, how they are made tamper-evident and how long\n")
	write("they are kept are #39, #43 and #44. Nothing in `internal/audit` writes a\n")
	write("record today, so every field above is a shape rather than a thing an\n")
	write("operator can query yet.\n")

	return b.String()
}
