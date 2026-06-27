// Package docgen generates a Unobin library reference manual.
package docgen

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/cloudboss/unobin/pkg/goschema"
)

const unobinModulePath = "github.com/cloudboss/unobin"

// Options configures a docs generation run.
type Options struct {
	RootDir     string
	OutDir      string
	ModulePath  string
	ImportAlias string
	Extra       []goschema.ModuleRoot
}

// Generate writes the generated reference manual pages.
func Generate(opts Options) error {
	rootDir := opts.RootDir
	if rootDir == "" {
		rootDir = "."
	}
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return err
	}
	outDir, err := docsOutputDir(rootAbs, opts.OutDir)
	if err != nil {
		return err
	}
	extra := append([]goschema.ModuleRoot(nil), opts.Extra...)
	if len(extra) == 0 {
		unobinRoot, err := findUnobinModuleRoot(rootAbs)
		if err != nil {
			return err
		}
		extra = append(extra, unobinRoot)
	}

	modulePath := opts.ModulePath
	if modulePath == "" {
		modulePath, err = currentModulePath(rootAbs)
		if err != nil {
			return err
		}
	}
	importAlias := opts.ImportAlias
	if importAlias == "" {
		importAlias = defaultImportAlias(modulePath)
	}

	schema, index, warnings, err := goschema.ReadWithIndex(rootAbs, extra...)
	if err != nil {
		return err
	}
	if len(warnings) > 0 {
		return fmt.Errorf("read schema: %s", strings.Join(warnings, "; "))
	}

	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	r := renderer{
		rootDir:     rootAbs,
		outDir:      outDir,
		modulePath:  modulePath,
		importAlias: importAlias,
		schema:      schema,
		index:       index,
		comments:    newCommentReader(rootAbs),
	}
	return r.renderAll()
}

func docsOutputDir(rootAbs string, outDir string) (string, error) {
	if outDir == "" {
		outDir = filepath.Join("docs", "reference")
	}
	var outAbs string
	if filepath.IsAbs(outDir) {
		outAbs = filepath.Clean(outDir)
	} else {
		outAbs = filepath.Join(rootAbs, outDir)
	}
	rel, err := filepath.Rel(rootAbs, outAbs)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("refusing to replace unsafe docs output directory")
	}
	return outAbs, nil
}

func findUnobinModuleRoot(dir string) (goschema.ModuleRoot, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", unobinModulePath)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return goschema.ModuleRoot{}, fmt.Errorf("locate unobin module: %w", err)
	}
	modDir := strings.TrimSpace(string(out))
	if modDir == "" {
		return goschema.ModuleRoot{}, errors.New("unobin module directory is empty")
	}
	return goschema.ModuleRoot{Path: unobinModulePath, Dir: modDir}, nil
}

func currentModulePath(dir string) (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}}")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read module path: %w", err)
	}
	modulePath := strings.TrimSpace(string(out))
	if modulePath == "" {
		return "", errors.New("module path is empty")
	}
	return modulePath, nil
}

func defaultImportAlias(modulePath string) string {
	name := pathBase(modulePath)
	name = strings.TrimPrefix(name, "unobin-library-")
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	alias := b.String()
	if alias == "" || unicode.IsDigit([]rune(alias)[0]) {
		return "lib"
	}
	return alias
}

func pathBase(path string) string {
	path = strings.TrimRight(path, "/")
	if path == "" {
		return ""
	}
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return path
	}
	return path[idx+1:]
}
