// Package docgen generates a Unobin library reference manual.
package docgen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/cloudboss/unobin/pkg/goschema"
	"github.com/cloudboss/unobin/pkg/runtime"
)

const unobinModulePath = "github.com/cloudboss/unobin"

var runGoCommand = func(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// Options configures a docs generation run.
type Options struct {
	RootDir        string
	OutDir         string
	PackageDir     string
	ModulePath     string
	ImportAlias    string
	CollectionPath string
	Libraries      []LibraryOptions
	Extra          []goschema.ModuleRoot
}

// LibraryOptions configures one library in a generated collection.
type LibraryOptions struct {
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	PackageDir  string `json:"package"`
	ModulePath  string `json:"module"`
	ImportAlias string `json:"alias"`
}

type collectionFile struct {
	Libraries []LibraryOptions `json:"libraries"`
}

type libraryDoc struct {
	title       string
	slug        string
	modulePath  string
	importAlias string
	rootDir     string
	outDir      string
	schema      *runtime.LibrarySchema
	index       *goschema.SourceIndex
	comments    *commentReader
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
	if opts.CollectionPath != "" {
		libs, err := readCollectionFile(rootAbs, opts.CollectionPath)
		if err != nil {
			return err
		}
		opts.Libraries = append(opts.Libraries, libs...)
	}
	if len(opts.Libraries) > 0 {
		if opts.PackageDir != "" || opts.ModulePath != "" || opts.ImportAlias != "" {
			return errors.New("collection cannot be combined with package, module, or alias")
		}
		return generateCollection(rootAbs, outDir, extra, opts.Libraries)
	}
	lib, err := readLibraryDoc(rootAbs, extra, LibraryOptions{
		PackageDir:  opts.PackageDir,
		ModulePath:  opts.ModulePath,
		ImportAlias: opts.ImportAlias,
	})
	if err != nil {
		return err
	}
	lib.outDir = outDir
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	return lib.renderer().renderAll()
}

func generateCollection(
	rootAbs string,
	outDir string,
	extra []goschema.ModuleRoot,
	libs []LibraryOptions,
) error {
	if len(libs) == 0 {
		return errors.New("collection has no libraries")
	}
	docs := make([]libraryDoc, 0, len(libs))
	seenSlugs := map[string]bool{}
	for _, opts := range libs {
		lib, err := readLibraryDoc(rootAbs, extra, opts)
		if err != nil {
			return err
		}
		if seenSlugs[lib.slug] {
			return fmt.Errorf("collection library slug %q is duplicated", lib.slug)
		}
		seenSlugs[lib.slug] = true
		lib.outDir = filepath.Join(outDir, lib.slug)
		docs = append(docs, lib)
	}
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	sharedConfig := sharedConfiguration(docs)
	if err := writeCollectionIndex(outDir, docs, sharedConfig >= 0); err != nil {
		return err
	}
	if err := writeCollectionSummary(outDir, docs, sharedConfig >= 0); err != nil {
		return err
	}
	if sharedConfig >= 0 {
		r := docs[sharedConfig].renderer()
		r.outDir = outDir
		r.configurationImports = configurationImports(docs)
		if err := r.writeConfiguration(); err != nil {
			return err
		}
	}
	for _, lib := range docs {
		if err := lib.renderer().renderCollectionLibrary(sharedConfig < 0); err != nil {
			return err
		}
	}
	return nil
}

func readLibraryDoc(
	rootAbs string,
	extra []goschema.ModuleRoot,
	opts LibraryOptions,
) (libraryDoc, error) {
	packageAbs, packageRel, err := libraryPackageDir(rootAbs, opts.PackageDir)
	if err != nil {
		return libraryDoc{}, err
	}
	modulePath := opts.ModulePath
	if modulePath == "" {
		modulePath, err = defaultModulePath(rootAbs, packageRel)
		if err != nil {
			return libraryDoc{}, err
		}
	}
	importAlias := opts.ImportAlias
	if importAlias == "" {
		importAlias = defaultImportAlias(modulePath)
	}
	title := opts.Title
	if title == "" {
		title = importAlias
	}
	slug := defaultLibrarySlug(opts, importAlias)
	schemaExtra := extraForPackage(rootAbs, packageAbs, extra)
	schema, index, warnings, err := goschema.ReadWithIndex(packageAbs, schemaExtra...)
	if err != nil {
		return libraryDoc{}, err
	}
	if len(warnings) > 0 {
		return libraryDoc{}, fmt.Errorf("read schema: %s", strings.Join(warnings, "; "))
	}
	return libraryDoc{
		title:       title,
		slug:        slug,
		modulePath:  modulePath,
		importAlias: importAlias,
		rootDir:     rootAbs,
		schema:      schema,
		index:       index,
		comments:    newCommentReader(rootAbs),
	}, nil
}

func readCollectionFile(rootAbs string, path string) ([]LibraryOptions, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootAbs, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read collection: %w", err)
	}
	var file collectionFile
	if err := json.Unmarshal(data, &file); err != nil {
		var libs []LibraryOptions
		if err := json.Unmarshal(data, &libs); err != nil {
			return nil, fmt.Errorf("parse collection: %w", err)
		}
		file.Libraries = libs
	}
	if len(file.Libraries) == 0 {
		return nil, errors.New("collection has no libraries")
	}
	return file.Libraries, nil
}

func libraryPackageDir(rootAbs string, packageDir string) (string, string, error) {
	if packageDir == "" {
		packageDir = "."
	}
	var packageAbs string
	if filepath.IsAbs(packageDir) {
		packageAbs = filepath.Clean(packageDir)
	} else {
		packageAbs = filepath.Join(rootAbs, packageDir)
	}
	rel, err := filepath.Rel(rootAbs, packageAbs)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errors.New("library package directory is outside root")
	}
	return packageAbs, filepath.ToSlash(rel), nil
}

func defaultModulePath(rootAbs string, packageRel string) (string, error) {
	modulePath, err := currentModulePath(rootAbs)
	if err != nil {
		return "", err
	}
	if packageRel == "." {
		return modulePath, nil
	}
	return modulePath + "//" + packageRel, nil
}

func defaultLibrarySlug(opts LibraryOptions, importAlias string) string {
	if opts.Slug != "" {
		return slugText(opts.Slug)
	}
	if opts.PackageDir != "" && opts.PackageDir != "." {
		return slugText(pathBase(filepath.ToSlash(opts.PackageDir)))
	}
	if opts.Title != "" {
		return slugText(opts.Title)
	}
	return slugText(importAlias)
}

func slugText(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '-' || r == '_' || r == '/' || unicode.IsSpace(r):
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "library"
	}
	return out
}

func extraForPackage(
	rootAbs string,
	packageAbs string,
	extra []goschema.ModuleRoot,
) []goschema.ModuleRoot {
	out := append([]goschema.ModuleRoot(nil), extra...)
	if packageAbs == rootAbs {
		return out
	}
	modulePath, err := currentModulePath(rootAbs)
	if err != nil || modulePath == "" {
		return out
	}
	root := goschema.ModuleRoot{Path: modulePath, Dir: rootAbs}
	for _, existing := range out {
		if existing.Path == root.Path && existing.Dir == root.Dir {
			return out
		}
	}
	return append(out, root)
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
	modDir, err := goModuleDir(dir, unobinModulePath)
	if err != nil {
		return goschema.ModuleRoot{}, err
	}
	if modDir == "" {
		if err := goDownloadModule(dir, unobinModulePath); err != nil {
			return goschema.ModuleRoot{}, err
		}
		modDir, err = goModuleDir(dir, unobinModulePath)
		if err != nil {
			return goschema.ModuleRoot{}, err
		}
	}
	if modDir == "" {
		return goschema.ModuleRoot{}, errors.New("unobin module directory is empty")
	}
	return goschema.ModuleRoot{Path: unobinModulePath, Dir: modDir}, nil
}

func goModuleDir(dir string, modulePath string) (string, error) {
	out, err := runGoCommand(dir, "list", "-m", "-f", "{{.Dir}}", modulePath)
	if err != nil {
		return "", fmt.Errorf("locate unobin module: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func goDownloadModule(dir string, modulePath string) error {
	out, err := runGoCommand(dir, "mod", "download", modulePath)
	if err != nil {
		return fmt.Errorf("download unobin module: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
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
