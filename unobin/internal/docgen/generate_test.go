package docgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudboss/unobin/pkg/goschema"
	"github.com/cloudboss/unobin/pkg/lang"
	"github.com/cloudboss/unobin/pkg/runtime"
	"github.com/cloudboss/unobin/pkg/typecheck"
)

func TestGenerateWritesKindReferenceFromGoDocs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"),
		"module example.com/unobin-library-compute\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(dir, "library.go"), `package library

import "github.com/cloudboss/unobin/pkg/runtime"

// Server manages a test server.
//
// Example:
//
//	resources: {
//	  app: compute.server { name: 'app' }
//	}
type Server struct {
	Name string `+"`"+`ub:"name"`+"`"+`
}

type ServerOutput struct {
	ID string `+"`"+`ub:"id"`+"`"+`
}

func Library() *runtime.Library {
	return &runtime.Library{
		Resources: map[string]runtime.ResourceRegistration{
			"server": runtime.MakeResource[Server, *ServerOutput, any](),
		},
	}
}
`)

	out := filepath.Join(dir, "docs", "reference")
	err := Generate(Options{
		RootDir: dir,
		OutDir:  out,
		Extra:   []goschema.ModuleRoot{{Path: "example.com/none", Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(out, "resources", "server.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	assertContains(t, text, "# compute.server resource")
	assertContains(t, text, "Example usage:\n\n```\nimports: {\n")
	assertContains(t, text, "  compute: 'example.com/unobin-library-compute'\n}")
	assertContains(t, text, "resources: {\n")
	assertContains(t, text, "Server manages a test server.")
	assertContains(t, text, "<div class=\"ub-fields\">")
	assertContains(t, text, "<p class=\"ub-field-name\"><strong>name</strong></p>")
	assertContains(t, text, "<span class=\"ub-badge ub-badge--required\">required</span>")
	assertContains(t, text, "<p class=\"ub-field-type\"><code>string</code></p>")
	assertContains(t, text, "<p class=\"ub-field-name\"><strong>id</strong></p>")
	assertNotContains(t, text, "Required: <code>false</code>")
	assertNotContains(t, text, "Sensitive: <code>false</code>")
	assertNotContains(t, text, "| Field | Type |")
}

func TestGenerateResolvesRelativeOutDirAgainstRoot(t *testing.T) {
	root := t.TempDir()
	cwd := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"),
		"module example.com/unobin-library-app\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(root, "library.go"), `package library

import "github.com/cloudboss/unobin/pkg/runtime"

type Server struct {
	Name string `+"`"+`ub:"name"`+"`"+`
}

type ServerOutput struct {
	ID string `+"`"+`ub:"id"`+"`"+`
}

func Library() *runtime.Library {
	return &runtime.Library{
		Resources: map[string]runtime.ResourceRegistration{
			"server": runtime.MakeResource[Server, *ServerOutput, any](),
		},
	}
}
`)
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	}()

	err = Generate(Options{
		RootDir: root,
		OutDir:  filepath.Join("docs", "reference"),
		Extra:   []goschema.ModuleRoot{{Path: "example.com/none", Dir: root}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "docs", "reference", "index.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "docs", "reference", "index.md")); !os.IsNotExist(err) {
		t.Fatalf("expected no generated docs under cwd, got %v", err)
	}
}

func TestGenerateRejectsOutputOutsideRoot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/lib\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(root, "library.go"), `package library

import "github.com/cloudboss/unobin/pkg/runtime"

func Library() *runtime.Library { return &runtime.Library{} }
`)

	err := Generate(Options{
		RootDir: root,
		OutDir:  filepath.Join("..", "reference"),
		Extra:   []goschema.ModuleRoot{{Path: "example.com/none", Dir: root}},
	})
	if err == nil {
		t.Fatal("expected unsafe output directory error")
	}
	assertContains(t, err.Error(), "refusing to replace unsafe docs output directory")
}

func TestGenerateOmitsConfigurationWhenLibraryHasNoConfig(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/lib\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(dir, "library.go"), `package library

import "github.com/cloudboss/unobin/pkg/runtime"

func Library() *runtime.Library { return &runtime.Library{} }
`)

	out := filepath.Join(dir, "docs", "reference")
	err := Generate(Options{
		RootDir: dir,
		OutDir:  out,
		Extra:   []goschema.ModuleRoot{{Path: "example.com/none", Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}

	index, err := os.ReadFile(filepath.Join(out, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertNotContains(t, string(index), "Configuration")
	if _, err := os.Stat(filepath.Join(out, "configuration.md")); !os.IsNotExist(err) {
		t.Fatalf("expected no configuration page, got %v", err)
	}
}

func TestGenerateWritesFunctionReference(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"),
		"module example.com/unobin-library-std\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(dir, "library.go"), `package library

import (
	"strings"

	"github.com/cloudboss/unobin/pkg/runtime"
)

func join(parts []string, sep string) (string, error) {
	return strings.Join(parts, sep), nil
}

func Library() *runtime.Library {
	return &runtime.Library{
		Functions: map[string]runtime.FunctionType{
			"join": runtime.MakeFunc("join", "Join strings.", join),
		},
	}
}
`)

	out := filepath.Join(dir, "docs", "reference")
	err := Generate(Options{
		RootDir: dir,
		OutDir:  out,
		Extra:   []goschema.ModuleRoot{{Path: "example.com/none", Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}

	index, err := os.ReadFile(filepath.Join(out, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(index), "- [Functions](functions/) (1)")
	assertNotContains(t, string(index), "Resources")

	got, err := os.ReadFile(filepath.Join(out, "functions", "join.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	assertContains(t, text, "# std.join function")
	assertContains(t, text, "Source: `library.go:9`")
	assertContains(t, text, "std.join(list(string), string) string")
}

func TestWriteConfigurationUsesFieldCards(t *testing.T) {
	dir := t.TempDir()
	schema := &runtime.LibrarySchema{
		ConfigurationFields: []typecheck.ObjectField{
			{Name: "region", Type: typecheck.TString(), Optional: true},
			{Name: "retry-mode", Type: typecheck.TString(), Optional: true},
			{Name: "endpoints", Type: typecheck.TObject([]typecheck.ObjectField{
				{Name: "s3", Type: typecheck.TString(), Optional: true},
				{Name: "sts", Type: typecheck.TString(), Optional: true},
			})},
		},
		ConfigurationDefaults: []lang.DefaultSpec{
			{Field: "input.retry-mode", Value: "'standard'"},
		},
	}

	err := renderer{outDir: dir, schema: schema, modulePath: "example.com/lib"}.writeConfiguration()
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	assertContains(t, text, "`library-config('example.com/lib')`")
	assertContains(t, text, "## Fields\n\n<div class=\"ub-fields\">")
	assertContains(t, text, "<p class=\"ub-field-name\"><strong>region</strong></p>")
	assertContains(t, text, "<p class=\"ub-field-type\"><code>string</code></p>")
	assertContains(t, text, "<p class=\"ub-field-name\"><strong>retry-mode</strong></p>")
	assertContains(t, text, "<span class=\"ub-default-label\">default</span>")
	assertContains(t, text, "<code class=\"ub-default-value\">standard</code>")
	assertContains(t, text, "<p class=\"ub-field-name\"><strong>endpoints</strong></p>")
	assertContains(t, text, "<summary><code>object</code></summary>")
	assertContains(t, text, strings.Join([]string{
		"<pre class=\"ub-field-type-block\"><code>object({",
		"  s3: optional(string)",
		"  sts: optional(string)",
		"})</code></pre>",
	}, "\n"))
	assertNotContains(t, text, "Required: <code>false</code>")
	assertNotContains(t, text, "Sensitive: <code>false</code>")
}

func TestWriteFieldTableCompactsScalarFields(t *testing.T) {
	fields := map[string]typecheck.Type{
		"config": typecheck.TObject([]typecheck.ObjectField{
			{Name: "enabled", Type: typecheck.TBoolean()},
			{Name: "name", Type: typecheck.TString()},
		}),
		"description": typecheck.TString(),
		"maybe-config": typecheck.TOptional(typecheck.TObject([]typecheck.ObjectField{
			{Name: "enabled", Type: typecheck.TBoolean()},
		})),
		"name":   typecheck.TString(),
		"secret": typecheck.TString(),
	}
	defaults := []lang.DefaultSpec{
		{Field: "input.config", Value: "{ enabled: true }"},
		{Field: "input.description", Optional: true},
		{Field: "input.name", Value: "'server'"},
	}

	var b strings.Builder
	renderer{}.writeFieldTable(&b, "Inputs", fields, nil, true, defaults, []string{"secret"}, nil)
	got := b.String()

	assertContains(t, got, "<section class=\"ub-field ub-field--simple\">")
	assertContains(t, got, "<p class=\"ub-field-name\"><strong>description</strong></p>")
	assertContains(t, got, "<p class=\"ub-field-type\"><code>string</code></p>")
	assertContains(t, got, "<p class=\"ub-field-name\"><strong>secret</strong></p>")
	assertContains(t, got, "<span class=\"ub-badge ub-badge--sensitive\">sensitive</span>")
	assertContains(t, got, "<p class=\"ub-field-name\"><strong>name</strong></p>")
	assertContains(t, got, "<span class=\"ub-default-label\">default</span>")
	assertContains(t, got, "<code class=\"ub-default-value\">server</code>")
	assertContains(t, got,
		"<pre class=\"ub-default-value-block\"><code>{ enabled: true }</code></pre>")
	assertContains(t, got, "<summary><code>object</code></summary>")
	assertContains(t, got, "<summary><code>optional(object)</code></summary>")
	assertContains(t, got, strings.Join([]string{
		"<pre class=\"ub-field-type-block\"><code>object({",
		"  enabled: boolean",
		"  name: string",
		"})</code></pre>",
	}, "\n"))
	assertNotContains(t, got, "Required: <code>false</code>")
	assertNotContains(t, got, "Sensitive: <code>false</code>")
	assertNotContains(t, got, "| Field | Type |")
}

func TestWriteCategoryIndexUsesSingularKind(t *testing.T) {
	dir := t.TempDir()
	cat := category{Title: "Actions", Noun: "action", Dir: "actions"}

	err := renderer{outDir: dir, importAlias: "lib"}.writeCategoryIndex(cat, []string{"invoke"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "actions", "index.md"))
	if err != nil {
		t.Fatal(err)
	}

	assertContains(t, string(got), "This library exports 1 action kind.")
}

func TestWriteKindOrdersInputConstraintsBeforeOutputs(t *testing.T) {
	dir := t.TempDir()
	ts := &runtime.TypeSchema{
		Inputs: map[string]typecheck.Type{
			"name": typecheck.TString(),
		},
		Outputs: map[string]typecheck.Type{
			"id": typecheck.TString(),
		},
		Constraints: []lang.ConstraintSpec{
			{Kind: "at-most-one-of", Fields: []string{"input.name", "input.alias"}},
		},
	}
	cat := category{
		Kind:  "resource",
		Noun:  "resource",
		Dir:   "resources",
		Block: "resources",
	}

	err := renderer{
		outDir:      dir,
		modulePath:  "example.com/lib",
		importAlias: "lib",
	}.writeKind(cat, "server", ts)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "resources", "server.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)

	assertContains(t, text, "Example usage:\n\n```\nimports: {\n")
	assertContains(t, text, "  lib: 'example.com/lib'")
	assertContains(t, text, "## Input Constraints")
	assertBefore(t, text, "## Inputs", "## Input Constraints")
	assertBefore(t, text, "## Input Constraints", "## Outputs")
}

func TestWriteConstraintsRendersGroupedCards(t *testing.T) {
	constraints := []lang.ConstraintSpec{
		{
			Kind: "predicate",
			When: "(input.protocol == 'HTTP' || input.protocol == 'TCP' || " +
				"input.protocol == 'UDP' || input.protocol == 'TCP_UDP' || " +
				"input.protocol == 'GENEVE' || input.protocol == 'QUIC' || " +
				"input.protocol == 'TCP_QUIC')",
			Require: "(input.ssl-policy == null) && (input.certificate-arn == null) && " +
				"(input.alpn-policy == null)",
			Message: "TLS fields require a TLS listener.",
		},
		{
			Kind:    "predicate",
			ForEach: "input.default-action",
			When:    "true",
			Require: "((@each.value.target-group-arn != null) || (@each.value.forward != null)) && " +
				"(@each.value.redirect == null)",
			Message: "a forward action takes target-group-arn or a forward block only",
		},
		{
			Kind:    "predicate",
			ForEach: "input.route-settings",
			When:    "@each.value.logging-level != null",
			Require: "@each.value.logging-level == 'ERROR' || " +
				"@each.value.logging-level == 'INFO' || " +
				"@each.value.logging-level == 'OFF'",
			Message: "route-settings logging-level must be ERROR, INFO, or OFF",
		},
		{
			Kind:    "predicate",
			ForEach: "input.emit-system-fields",
			Require: "@each.value == '@cloud.account' || @each.value == '@cloud.region'",
			Message: "emit-system-fields entries must be @cloud.account or @cloud.region",
		},
		{
			Kind:   "at-most-one-of",
			Fields: []string{"input.a", "input.b"},
		},
		{
			Kind:   "exactly-one-of",
			Fields: []string{"input.carrier-gateway-id", "input.gateway-id", "input.nat-gateway-id"},
		},
		{
			Kind:   "required-with",
			Fields: []string{"input.certificate-body", "input.private-key"},
		},
		{
			Kind: "forbidden-with",
			Fields: []string{
				"input.domain-name",
				"input.certificate-body",
				"input.private-key",
				"input.certificate-chain",
			},
		},
	}

	var b strings.Builder
	renderer{}.writeConstraints(&b, constraints)
	got := b.String()

	assertContains(t, got, "## Input Constraints")
	assertContains(t, got, "<div class=\"ub-constraints\">")
	assertContains(t, got, "<h3 class=\"ub-constraint-group-title\">Protocol rules</h3>")
	assertContains(t, got, "<p class=\"ub-constraint-summary\">TLS fields require a TLS listener.</p>")
	assertContains(t, got, "<summary>Rule logic</summary>")
	assertContains(t, got, "<dt>When</dt>")
	assertContains(t, got, "<code>input.protocol == &#39;HTTP&#39;</code><br>")
	assertContains(t, got, "<code>|| input.protocol == &#39;TCP_QUIC&#39;</code>")
	assertContains(t, got, "<dt>Require</dt>")
	assertContains(t, got, "<code>input.ssl-policy == null</code><br>")
	assertContains(t, got, "<code>&amp;&amp; input.alpn-policy == null</code>")
	assertContains(t, got, "<h3 class=\"ub-constraint-group-title\">Default action rules</h3>")
	assertContains(t, got, "<dt>For each</dt>")
	assertContains(t, got, "<code>input.default-action</code>")
	assertContains(t, got, "<h3 class=\"ub-constraint-group-title\">Route settings rules</h3>")
	assertContains(t, got, strings.Join([]string{
		"<strong>route-settings</strong> logging-level must be <code>ERROR</code>,",
		"<code>INFO</code>, or <code>OFF</code>.",
	}, " "))
	assertContains(t, got, "<h3 class=\"ub-constraint-group-title\">Emit system fields rules</h3>")
	assertContains(t, got, strings.Join([]string{
		"<strong>emit-system-fields</strong> entries must be <code>@cloud.account</code>",
		"or <code>@cloud.region</code>.",
	}, " "))
	assertContains(t, got, "<h3 class=\"ub-constraint-group-title\">Field combinations</h3>")
	assertContains(t, got, "At most one of <strong>a</strong> or <strong>b</strong>.")
	assertContains(t, got, strings.Join([]string{
		"Exactly one of <strong>carrier-gateway-id</strong>,",
		"<strong>gateway-id</strong>, or <strong>nat-gateway-id</strong>.",
	}, " "))
	assertContains(t, got,
		"Required together: <strong>certificate-body</strong> and <strong>private-key</strong>.")
	assertContains(t, got, strings.Join([]string{
		"Forbidden together: <strong>domain-name</strong>,",
		"<strong>certificate-body</strong>, <strong>private-key</strong>, and",
		"<strong>certificate-chain</strong>.",
	}, " "))
	assertNotContains(t, got, "!!! constraint")
	assertNotContains(t, got, "Conditional requirement")
	assertNotContains(t, got, "```")
	assertNotContains(t, got, "when: "+constraints[0].When+"; require:")
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected %q to contain %q", haystack, needle)
	}
}

func assertNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("expected %q not to contain %q", haystack, needle)
	}
}

func assertBefore(t *testing.T, haystack, left, right string) {
	t.Helper()
	leftIndex := strings.Index(haystack, left)
	rightIndex := strings.Index(haystack, right)
	if leftIndex == -1 || rightIndex == -1 || leftIndex >= rightIndex {
		t.Fatalf("expected %q to appear before %q in %q", left, right, haystack)
	}
}
