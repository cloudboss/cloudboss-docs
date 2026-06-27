package docgen

import (
	"go/ast"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"

	"github.com/cloudboss/unobin/pkg/goschema"
)

type commentReader struct {
	rootDir string
	files   map[string]*fileComments
}

type fileComments struct {
	types map[int]typeComment
}

type typeComment struct {
	Name   string
	Doc    string
	Fields map[string]string
	Path   string
	Line   int
}

func newCommentReader(rootDir string) *commentReader {
	return &commentReader{
		rootDir: rootDir,
		files:   map[string]*fileComments{},
	}
}

func (r *commentReader) typeAt(loc goschema.GoLocation) (typeComment, error) {
	if loc.Path == "" {
		return typeComment{}, nil
	}
	path := loc.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.rootDir, path)
	}
	fc, ok := r.files[path]
	if !ok {
		parsed, err := parseFileComments(path)
		if err != nil {
			return typeComment{}, err
		}
		r.files[path] = parsed
		fc = parsed
	}
	return fc.types[loc.Offset], nil
}

func parseFileComments(path string) (*fileComments, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	out := &fileComments{types: map[int]typeComment{}}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typ, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			pos := fset.Position(typ.Name.Pos())
			out.types[pos.Offset] = typeComment{
				Name:   typ.Name.Name,
				Doc:    renderGoDoc(typeDoc(gen, typ)),
				Fields: fieldComments(typ),
				Path:   path,
				Line:   pos.Line,
			}
		}
	}
	return out, nil
}

func typeDoc(gen *ast.GenDecl, typ *ast.TypeSpec) string {
	if typ.Doc != nil {
		return typ.Doc.Text()
	}
	if len(gen.Specs) == 1 && gen.Doc != nil {
		return gen.Doc.Text()
	}
	return ""
}

func fieldComments(typ *ast.TypeSpec) map[string]string {
	st, ok := typ.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return nil
	}
	out := map[string]string{}
	for _, f := range st.Fields.List {
		name := fieldName(f)
		if name == "" || f.Doc == nil {
			continue
		}
		out[name] = renderGoDoc(f.Doc.Text())
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func fieldName(f *ast.Field) string {
	if f.Tag != nil {
		tag := strings.Trim(f.Tag.Value, "`")
		name := strings.Split(reflect.StructTag(tag).Get("ub"), ",")[0]
		if name != "" && name != "-" {
			return name
		}
	}
	if len(f.Names) == 0 {
		return ""
	}
	return goNameToKebab(f.Names[0].Name)
}

func renderGoDoc(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var parser comment.Parser
	var printer comment.Printer
	return strings.TrimSpace(unescapeMarkdownEscapes(string(printer.Markdown(parser.Parse(text)))))
}

func goNameToKebab(s string) string {
	var b strings.Builder
	var prev rune
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 && (unicode.IsLower(prev) || unicode.IsDigit(prev) || nextIsLower(s, i)) {
				b.WriteByte('-')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
		prev = r
	}
	return b.String()
}

func nextIsLower(s string, byteIndex int) bool {
	for i, r := range s {
		if i <= byteIndex {
			continue
		}
		return unicode.IsLower(r)
	}
	return false
}
