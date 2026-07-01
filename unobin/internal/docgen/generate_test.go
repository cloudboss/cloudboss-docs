package docgen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
			"server": runtime.MakeResource[Server, *ServerOutput, runtime.NoConfig, *Server](),
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
	assertContains(t, string(index), "# Overview")
	summary, err := os.ReadFile(filepath.Join(out, "SUMMARY.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(summary), "* [Overview](index.md)")

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
			"server": runtime.MakeResource[Server, *ServerOutput, runtime.NoConfig, *Server](),
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

func TestGenerateWritesPointerOptionalInputsAndNullableDefaults(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"),
		"module example.com/unobin-library-compute\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(dir, "library.go"), `package library

import (
	"github.com/cloudboss/unobin/pkg/defaults"
	"github.com/cloudboss/unobin/pkg/runtime"
)

type Server struct {
	Name       string
	Tags       map[string]string
	Names      []string
	MaybeTags  *map[string]string `+"`"+`ub:"maybe-tags"`+"`"+`
	MaybeNames *[]string          `+"`"+`ub:"maybe-names"`+"`"+`
	Profile    *string
}

type ServerOutput struct {
	ID string
}

func (s Server) Defaults() []defaults.Default {
	return []defaults.Default{
		defaults.NullableValue(s.MaybeTags, map[string]string{"env": "test"}),
		defaults.NullableValue(s.MaybeNames, []string{"web"}),
		defaults.NullableValue(s.Profile, "dev"),
	}
}

func Library() *runtime.Library {
	return &runtime.Library{
		Resources: map[string]runtime.ResourceRegistration{
			"server": runtime.MakeResource[Server, *ServerOutput, runtime.NoConfig, *Server](),
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
	assertFieldCardContains(t, text, "tags", "<code>map(string)</code>")
	assertFieldCardContains(t, text, "tags", "ub-badge--required")
	assertFieldCardContains(t, text, "names", "<code>list(string)</code>")
	assertFieldCardContains(t, text, "names", "ub-badge--required")
	assertFieldCardContains(t, text, "maybe-tags", "<code>optional(map(string))</code>")
	assertFieldCardContains(t, text, "maybe-tags", "{ env: &#39;test&#39; }")
	assertFieldCardNotContains(t, text, "maybe-tags", "ub-badge--required")
	assertFieldCardContains(t, text, "maybe-names", "<code>optional(list(string))</code>")
	assertFieldCardContains(t, text, "maybe-names", "[&#39;web&#39;]")
	assertFieldCardNotContains(t, text, "maybe-names", "ub-badge--required")
	assertFieldCardContains(t, text, "profile", "<code>optional(string)</code>")
	assertFieldCardContains(t, text, "profile", "<code class=\"ub-default-value\">dev</code>")
	assertFieldCardNotContains(t, text, "profile", "ub-badge--required")
}

func TestGenerateWritesPlainConfigurationDefaultsAndConstraints(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/lib\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(dir, "library.go"), `package library

import (
	"github.com/cloudboss/unobin/pkg/constraint"
	"github.com/cloudboss/unobin/pkg/defaults"
	"github.com/cloudboss/unobin/pkg/runtime"
	"github.com/cloudboss/unobin/pkg/sdk/cfg"
)

type Configuration struct {
	Region      string
	Profile     *string
	Tags        map[string]string
	MaybeTags   *map[string]string `+"`"+`ub:"maybe-tags"`+"`"+`
	MaxAttempts int64              `+"`"+`ub:"max-attempts"`+"`"+`
}

func (c Configuration) Defaults() []defaults.Default {
	return []defaults.Default{
		defaults.Value(c.MaxAttempts, int64(3)),
		defaults.NullableValue(c.Profile, "dev"),
		defaults.NullableValue(c.MaybeTags, map[string]string{"owner": "docs"}),
	}
}

func (c Configuration) Constraints() []constraint.Constraint {
	return []constraint.Constraint{
		constraint.Must(constraint.NotEmpty(c.Region)).Message("region is required"),
		constraint.Must(constraint.AtLeast(c.MaxAttempts, 1)).
			Message("max-attempts must be positive"),
	}
}

func Library() *runtime.Library {
	return &runtime.Library{
		Configuration: &cfg.ConfigurationType[*Configuration]{
			New: func() *Configuration {
				return &Configuration{}
			},
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
	assertContains(t, string(index), "- [Configuration](configuration.md)")

	got, err := os.ReadFile(filepath.Join(out, "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	assertFieldCardContains(t, text, "region", "<code>string</code>")
	assertFieldCardContains(t, text, "region", "ub-badge--required")
	assertFieldCardContains(t, text, "tags", "<code>map(string)</code>")
	assertFieldCardContains(t, text, "tags", "ub-badge--required")
	assertFieldCardContains(t, text, "profile", "<code>optional(string)</code>")
	assertFieldCardContains(t, text, "profile", "<code class=\"ub-default-value\">dev</code>")
	assertFieldCardNotContains(t, text, "profile", "ub-badge--required")
	assertFieldCardContains(t, text, "maybe-tags", "<code>optional(map(string))</code>")
	assertFieldCardContains(t, text, "maybe-tags", "{ owner: &#39;docs&#39; }")
	assertFieldCardNotContains(t, text, "maybe-tags", "ub-badge--required")
	assertFieldCardContains(t, text, "max-attempts", "<code>integer</code>")
	assertFieldCardContains(t, text, "max-attempts", "<code class=\"ub-default-value\">3</code>")
	assertFieldCardNotContains(t, text, "max-attempts", "ub-badge--required")
	assertContains(t, text, "## Configuration Constraints")
	assertContains(t, text,
		"<p class=\"ub-constraint-summary\"><strong>region</strong> is required.</p>")
	assertContains(t, text,
		"<p class=\"ub-constraint-summary\"><strong>max-attempts</strong> must be positive.</p>")
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
	assertContains(t, string(index), "- [Functions](functions/index.md) (1)")
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

func TestGenerateReadsLibraryPackageBelowRoot(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"),
		"module example.com/unobin-library-cloud\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(dir, "internal", "storage", "types.go"), `package storage

// Bucket stores objects.
type Bucket struct {
	Name string `+"`"+`ub:"name"`+"`"+`
}

type BucketOutput struct {
	ID string `+"`"+`ub:"id"`+"`"+`
}
`)
	writeTestFile(t, filepath.Join(dir, "s3", "library.go"), `package s3

import (
	"example.com/unobin-library-cloud/internal/storage"

	"github.com/cloudboss/unobin/pkg/runtime"
)

func Library() *runtime.Library {
	return &runtime.Library{
		Resources: map[string]runtime.ResourceRegistration{
			"bucket": runtime.MakeResource[
				storage.Bucket,
				*storage.BucketOutput,
				runtime.NoConfig,
				*storage.Bucket,
			](),
		},
	}
}
`)

	out := filepath.Join(dir, "docs", "reference")
	err := Generate(Options{
		RootDir:     dir,
		OutDir:      out,
		PackageDir:  "s3",
		ModulePath:  "example.com/unobin-library-cloud//s3",
		ImportAlias: "aws-s3",
		Extra:       []goschema.ModuleRoot{{Path: "example.com/none", Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(out, "resources", "bucket.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	assertContains(t, text, "# aws-s3.bucket resource")
	assertContains(t, text, "aws-s3: 'example.com/unobin-library-cloud//s3'")
	assertContains(t, text, "Source: `internal/storage/types.go:4`")
}

func TestGenerateCollectionWritesGroupedSummary(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"),
		"module example.com/unobin-library-cloud\n\ngo 1.26\n")
	writeLibraryPackage(t, dir, "s3", "Bucket", "bucket")
	writeLibraryPackage(t, dir, "ec2", "Instance", "instance")

	out := filepath.Join(dir, "docs", "reference")
	err := Generate(Options{
		RootDir: dir,
		OutDir:  out,
		Libraries: []LibraryOptions{
			{
				Title:       "S3",
				PackageDir:  "s3",
				ModulePath:  "example.com/unobin-library-cloud//s3",
				ImportAlias: "aws-s3",
			},
			{
				Title:       "EC2",
				PackageDir:  "ec2",
				ModulePath:  "example.com/unobin-library-cloud//ec2",
				ImportAlias: "aws-ec2",
			},
		},
		Extra: []goschema.ModuleRoot{{Path: "example.com/none", Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}

	index, err := os.ReadFile(filepath.Join(out, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(index), "- [S3](s3/index.md) - `aws-s3: '")
	assertContains(t, string(index), "- [EC2](ec2/index.md) - `aws-ec2: '")

	summary, err := os.ReadFile(filepath.Join(out, "SUMMARY.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(summary), "* [Overview](index.md)")
	assertContains(t, string(summary), strings.Join([]string{
		"* S3",
		"    * [Overview](s3/index.md)",
		"    * Resources",
		"        * [bucket](s3/resources/bucket.md)",
	}, "\n"))
	assertContains(t, string(summary), strings.Join([]string{
		"* EC2",
		"    * [Overview](ec2/index.md)",
		"    * Resources",
		"        * [instance](ec2/resources/instance.md)",
	}, "\n"))

	if _, err := os.Stat(filepath.Join(out, "s3", "SUMMARY.md")); !os.IsNotExist(err) {
		t.Fatalf("expected no nested SUMMARY.md, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "s3", "resources", "SUMMARY.md")); !os.IsNotExist(err) {
		t.Fatalf("expected no nested resources SUMMARY.md, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "s3", "resources", "index.md")); !os.IsNotExist(err) {
		t.Fatalf("expected no nested resources index.md, got %v", err)
	}

	serviceIndex, err := os.ReadFile(filepath.Join(out, "s3", "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(serviceIndex), "# S3")
	assertContains(t, string(serviceIndex), "aws-s3: 'example.com/unobin-library-cloud//s3'")
	assertContains(t, string(serviceIndex), strings.Join([]string{
		"- Resources (1)",
		"  - [`aws-s3.bucket`](resources/bucket.md)",
	}, "\n"))
	assertNotContains(t, string(serviceIndex), "resources/index.md")

	kind, err := os.ReadFile(filepath.Join(out, "s3", "resources", "bucket.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(kind), "# aws-s3.bucket resource")
	assertContains(t, string(kind), "aws-s3: 'example.com/unobin-library-cloud//s3'")
}

func TestGenerateCollectionWritesPackageConfigurationPages(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"),
		"module example.com/unobin-library-cloud\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(dir, "config", "config.go"), `package config

import "github.com/cloudboss/unobin/pkg/sdk/cfg"

type Configuration struct {
	Region string `+"`"+`ub:"region"`+"`"+`
}

func LibraryConfiguration() *cfg.ConfigurationType[*Configuration] {
	return &cfg.ConfigurationType[*Configuration]{
		New: func() *Configuration {
			return &Configuration{}
		},
	}
}
`)
	writeLibraryPackageWithConfig(t, dir, "s3", "Bucket", "bucket")
	writeLibraryPackageWithConfig(t, dir, "ec2", "Instance", "instance")

	out := filepath.Join(dir, "docs", "reference")
	err := Generate(Options{
		RootDir: dir,
		OutDir:  out,
		Libraries: []LibraryOptions{
			{
				Title:       "S3",
				PackageDir:  "s3",
				ModulePath:  "example.com/unobin-library-cloud//s3",
				ImportAlias: "aws-s3",
			},
			{
				Title:       "EC2",
				PackageDir:  "ec2",
				ModulePath:  "example.com/unobin-library-cloud//ec2",
				ImportAlias: "aws-ec2",
			},
		},
		Extra: []goschema.ModuleRoot{{Path: "example.com/none", Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}

	index, err := os.ReadFile(filepath.Join(out, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertNotContains(t, string(index), "- [Configuration](configuration.md)")

	summary, err := os.ReadFile(filepath.Join(out, "SUMMARY.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertNotContains(t, string(summary), "* [Configuration](configuration.md)")
	assertContains(t, string(summary), "    * [Configuration](s3/configuration.md)")
	assertContains(t, string(summary), "    * [Configuration](ec2/configuration.md)")

	if _, err := os.Stat(filepath.Join(out, "configuration.md")); !os.IsNotExist(err) {
		t.Fatalf("expected no collection configuration page, got %v", err)
	}

	s3Config, err := os.ReadFile(filepath.Join(out, "s3", "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	s3Text := string(s3Config)
	assertContains(t, s3Text, "`library-config('example.com/unobin-library-cloud//config')`")
	assertContains(t, s3Text, strings.Join([]string{
		"Example usage:",
		"",
		"```",
		"imports: {",
		"  aws-s3: 'example.com/unobin-library-cloud//s3'",
		"}",
		"",
		"inputs: {",
		"  aws-config: {",
		"    type: library-config('example.com/unobin-library-cloud//config')",
		"  }",
		"}",
		"",
		"library-configs: {",
		"  aws-s3: input.aws-config",
		"}",
		"```",
	}, "\n"))
	assertNotContains(t, s3Text, "These libraries use the same configuration schema")
	assertNotContains(t, s3Text, "`library-config('example.com/unobin-library-cloud//s3')`")

	ec2Config, err := os.ReadFile(filepath.Join(out, "ec2", "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	ec2Text := string(ec2Config)
	assertContains(t, ec2Text, "`library-config('example.com/unobin-library-cloud//config')`")
	assertNotContains(t, ec2Text, "`library-config('example.com/unobin-library-cloud//ec2')`")
}

func TestGenerateReadsCollectionFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"),
		"module example.com/unobin-library-cloud\n\ngo 1.26\n")
	writeLibraryPackage(t, dir, "s3", "Bucket", "bucket")
	writeTestFile(t, filepath.Join(dir, "docs-libraries.json"), `{
  "libraries": [
    {
      "title": "S3",
      "package": "s3",
      "module": "example.com/unobin-library-cloud//s3",
      "alias": "aws-s3"
    }
  ]
}
`)

	out := filepath.Join(dir, "docs", "reference")
	err := Generate(Options{
		RootDir:        dir,
		OutDir:         out,
		CollectionPath: "docs-libraries.json",
		Extra:          []goschema.ModuleRoot{{Path: "example.com/none", Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(out, "s3", "resources", "bucket.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(got), "# aws-s3.bucket resource")
}

func TestFindUnobinModuleRootDownloadsMissingModule(t *testing.T) {
	dir := t.TempDir()
	moduleDir := filepath.Join(dir, "unobin")
	downloaded := false
	var calls [][]string
	oldRunGoCommand := runGoCommand
	runGoCommand = func(gotDir string, args ...string) ([]byte, error) {
		if gotDir != dir {
			return nil, errors.New("unexpected command directory")
		}
		calls = append(calls, append([]string(nil), args...))
		switch {
		case slices.Equal(args, []string{"list", "-m", "-f", "{{.Dir}}", unobinModulePath}):
			if downloaded {
				return []byte(moduleDir + "\n"), nil
			}
			return []byte("\n"), nil
		case slices.Equal(args, []string{"mod", "download", unobinModulePath}):
			downloaded = true
			return nil, nil
		default:
			return nil, errors.New("unexpected go command")
		}
	}
	t.Cleanup(func() { runGoCommand = oldRunGoCommand })

	root, err := findUnobinModuleRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root != (goschema.ModuleRoot{Path: unobinModulePath, Dir: moduleDir}) {
		t.Fatalf("unexpected module root: %#v", root)
	}
	wantCalls := [][]string{
		{"list", "-m", "-f", "{{.Dir}}", unobinModulePath},
		{"mod", "download", unobinModulePath},
		{"list", "-m", "-f", "{{.Dir}}", unobinModulePath},
	}
	if !equalStringLists(calls, wantCalls) {
		t.Fatalf("unexpected go calls: %#v", calls)
	}
}

func equalStringLists(left [][]string, right [][]string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !slices.Equal(left[i], right[i]) {
			return false
		}
	}
	return true
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

	err := renderer{
		outDir:      dir,
		schema:      schema,
		modulePath:  "example.com/lib",
		importAlias: "lib",
	}.writeConfiguration()
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	assertContains(t, text, "`library-config('example.com/lib')`")
	assertContains(t, text, strings.Join([]string{
		"Example usage:",
		"",
		"```",
		"imports: {",
		"  lib: 'example.com/lib'",
		"}",
		"",
		"inputs: {",
		"  lib-config: {",
		"    type: library-config('example.com/lib')",
		"  }",
		"}",
		"",
		"library-configs: {",
		"  lib: input.lib-config",
		"}",
		"```",
	}, "\n"))
	assertContains(t, text, "## Fields\n\n<div class=\"ub-fields\">")
	assertFieldCardContains(t, text, "region", "<code>optional(string)</code>")
	assertFieldCardNotContains(t, text, "region", "ub-badge--required")
	assertFieldCardContains(t, text, "retry-mode", "<code>optional(string)</code>")
	assertFieldCardContains(t, text, "retry-mode", "<span class=\"ub-default-label\">default</span>")
	assertFieldCardContains(t, text, "retry-mode", "<code class=\"ub-default-value\">standard</code>")
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
			ForEach: "input.cpu-configurations ?? []",
			When:    "true",
			Require: "@each.value.cpu != null",
			Message: "cpu-configurations entries must include cpu",
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

	want := "\n" + strings.Join([]string{
		"## Input Constraints",
		"",
		"<div class=\"ub-constraints\">",
		"<section class=\"ub-constraint-group\" data-constraint-group=\"protocol\">",
		"<h3 class=\"ub-constraint-group-title\">Protocol rules</h3>",
		"<div class=\"ub-constraint\">",
		"<p class=\"ub-constraint-summary\">TLS fields require a TLS listener.</p>",
		"<details class=\"ub-constraint-logic\">",
		"<summary>Rule logic</summary>",
		"<dl>",
		"<dt>When</dt>",
		"<dd><code>input.protocol == &#39;HTTP&#39;</code><br>",
		"<code>|| input.protocol == &#39;TCP&#39;</code><br>",
		"<code>|| input.protocol == &#39;UDP&#39;</code><br>",
		"<code>|| input.protocol == &#39;TCP_UDP&#39;</code><br>",
		"<code>|| input.protocol == &#39;GENEVE&#39;</code><br>",
		"<code>|| input.protocol == &#39;QUIC&#39;</code><br>",
		"<code>|| input.protocol == &#39;TCP_QUIC&#39;</code></dd>",
		"<dt>Require</dt>",
		"<dd><code>input.ssl-policy == null</code><br>",
		"<code>&amp;&amp; input.certificate-arn == null</code><br>",
		"<code>&amp;&amp; input.alpn-policy == null</code></dd>",
		"</dl>",
		"</details>",
		"</div>",
		"</section>",
		"<section class=\"ub-constraint-group\" data-constraint-group=\"default-action\">",
		"<h3 class=\"ub-constraint-group-title\">Default action rules</h3>",
		"<div class=\"ub-constraint\">",
		"<p class=\"ub-constraint-summary\">" +
			"a forward action takes target-group-arn or a forward block only.</p>",
		"<details class=\"ub-constraint-logic\">",
		"<summary>Rule logic</summary>",
		"<dl>",
		"<dt>For each</dt>",
		"<dd><code>input.default-action</code></dd>",
		"<dt>Require</dt>",
		"<dd><code>((@each.value.target-group-arn != null) || " +
			"(@each.value.forward != null))</code><br>",
		"<code>&amp;&amp; @each.value.redirect == null</code></dd>",
		"</dl>",
		"</details>",
		"</div>",
		"</section>",
		"<section class=\"ub-constraint-group\" data-constraint-group=\"route-settings\">",
		"<h3 class=\"ub-constraint-group-title\">Route settings rules</h3>",
		"<div class=\"ub-constraint\">",
		"<p class=\"ub-constraint-summary\"><strong>route-settings</strong> " +
			"logging-level must be <code>ERROR</code>, <code>INFO</code>, or " +
			"<code>OFF</code>.</p>",
		"<details class=\"ub-constraint-logic\">",
		"<summary>Rule logic</summary>",
		"<dl>",
		"<dt>For each</dt>",
		"<dd><code>input.route-settings</code></dd>",
		"<dt>When</dt>",
		"<dd><code>@each.value.logging-level != null</code></dd>",
		"<dt>Require</dt>",
		"<dd><code>@each.value.logging-level == &#39;ERROR&#39;</code><br>",
		"<code>|| @each.value.logging-level == &#39;INFO&#39;</code><br>",
		"<code>|| @each.value.logging-level == &#39;OFF&#39;</code></dd>",
		"</dl>",
		"</details>",
		"</div>",
		"</section>",
		"<section class=\"ub-constraint-group\" data-constraint-group=\"cpu-configurations\">",
		"<h3 class=\"ub-constraint-group-title\">Cpu configurations rules</h3>",
		"<div class=\"ub-constraint\">",
		"<p class=\"ub-constraint-summary\"><strong>cpu-configurations</strong> " +
			"entries must include cpu.</p>",
		"<details class=\"ub-constraint-logic\">",
		"<summary>Rule logic</summary>",
		"<dl>",
		"<dt>For each</dt>",
		"<dd><code>input.cpu-configurations ?? []</code></dd>",
		"<dt>Require</dt>",
		"<dd><code>@each.value.cpu != null</code></dd>",
		"</dl>",
		"</details>",
		"</div>",
		"</section>",
		"<section class=\"ub-constraint-group\" data-constraint-group=\"emit-system-fields\">",
		"<h3 class=\"ub-constraint-group-title\">Emit system fields rules</h3>",
		"<div class=\"ub-constraint\">",
		"<p class=\"ub-constraint-summary\"><strong>emit-system-fields</strong> " +
			"entries must be <code>@cloud.account</code> or " +
			"<code>@cloud.region</code>.</p>",
		"<details class=\"ub-constraint-logic\">",
		"<summary>Rule logic</summary>",
		"<dl>",
		"<dt>For each</dt>",
		"<dd><code>input.emit-system-fields</code></dd>",
		"<dt>Require</dt>",
		"<dd><code>@each.value == &#39;@cloud.account&#39;</code><br>",
		"<code>|| @each.value == &#39;@cloud.region&#39;</code></dd>",
		"</dl>",
		"</details>",
		"</div>",
		"</section>",
		"<section class=\"ub-constraint-group\" data-constraint-group=\"field-combinations\">",
		"<h3 class=\"ub-constraint-group-title\">Field combinations</h3>",
		"<div class=\"ub-constraint\">",
		"<p class=\"ub-constraint-summary\">At most one of <strong>a</strong> " +
			"or <strong>b</strong>.</p>",
		"</div>",
		"<div class=\"ub-constraint\">",
		"<p class=\"ub-constraint-summary\">Exactly one of " +
			"<strong>carrier-gateway-id</strong>, <strong>gateway-id</strong>, or " +
			"<strong>nat-gateway-id</strong>.</p>",
		"</div>",
		"<div class=\"ub-constraint\">",
		"<p class=\"ub-constraint-summary\">Required together: " +
			"<strong>certificate-body</strong> and <strong>private-key</strong>.</p>",
		"</div>",
		"<div class=\"ub-constraint\">",
		"<p class=\"ub-constraint-summary\">Forbidden together: " +
			"<strong>domain-name</strong>, <strong>certificate-body</strong>, " +
			"<strong>private-key</strong>, and <strong>certificate-chain</strong>.</p>",
		"</div>",
		"</section>",
		"</div>",
	}, "\n") + "\n"
	if got != want {
		t.Fatalf("expected constraint output:\n%s\n\ngot:\n%s", want, got)
	}
}

func writeLibraryPackage(t *testing.T, root, dirName, typeName, kind string) {
	t.Helper()
	writeTestFile(t, filepath.Join(root, dirName, "library.go"), fmt.Sprintf(`package %[1]s

import "github.com/cloudboss/unobin/pkg/runtime"

type %[2]s struct {
	Name string `+"`"+`ub:"name"`+"`"+`
}

type %[2]sOutput struct {
	ID string `+"`"+`ub:"id"`+"`"+`
}

func Library() *runtime.Library {
	return &runtime.Library{
		Resources: map[string]runtime.ResourceRegistration{
			%[3]q: runtime.MakeResource[%[2]s, *%[2]sOutput, runtime.NoConfig, *%[2]s](),
		},
	}
}
`, dirName, typeName, kind))
}

func writeLibraryPackageWithConfig(t *testing.T, root, dirName, typeName, kind string) {
	t.Helper()
	writeTestFile(t, filepath.Join(root, dirName, "library.go"), fmt.Sprintf(`package %[1]s

import (
	cloudconfig "example.com/unobin-library-cloud/config"

	"github.com/cloudboss/unobin/pkg/runtime"
)

type %[2]s struct {
	Name string `+"`"+`ub:"name"`+"`"+`
}

type %[2]sOutput struct {
	ID string `+"`"+`ub:"id"`+"`"+`
}

func Library() *runtime.Library {
	return &runtime.Library{
		Configuration: cloudconfig.LibraryConfiguration(),
		Resources: map[string]runtime.ResourceRegistration{
			%[3]q: runtime.MakeResource[
				%[2]s,
				*%[2]sOutput,
				*cloudconfig.Configuration,
				*%[2]s,
			](),
		},
	}
}
`, dirName, typeName, kind))
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

func assertFieldCardContains(t *testing.T, text, field, needle string) {
	t.Helper()
	assertContains(t, fieldCardHTML(t, text, field), needle)
}

func assertFieldCardNotContains(t *testing.T, text, field, needle string) {
	t.Helper()
	assertNotContains(t, fieldCardHTML(t, text, field), needle)
}

func fieldCardHTML(t *testing.T, text, field string) string {
	t.Helper()
	marker := "<p class=\"ub-field-name\"><strong>" + field + "</strong></p>"
	nameIndex := strings.Index(text, marker)
	if nameIndex == -1 {
		t.Fatalf("expected field card for %q in %q", field, text)
	}
	start := strings.LastIndex(text[:nameIndex], "<section ")
	if start == -1 {
		t.Fatalf("expected field card section for %q in %q", field, text)
	}
	end := strings.Index(text[nameIndex:], "</section>")
	if end == -1 {
		t.Fatalf("expected field card end for %q in %q", field, text)
	}
	return text[start : nameIndex+end+len("</section>")]
}

func assertBefore(t *testing.T, haystack, left, right string) {
	t.Helper()
	leftIndex := strings.Index(haystack, left)
	rightIndex := strings.Index(haystack, right)
	if leftIndex == -1 || rightIndex == -1 || leftIndex >= rightIndex {
		t.Fatalf("expected %q to appear before %q in %q", left, right, haystack)
	}
}
