package docgen

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudboss/unobin/pkg/goschema"
	"github.com/cloudboss/unobin/pkg/lang"
	"github.com/cloudboss/unobin/pkg/runtime"
	"github.com/cloudboss/unobin/pkg/typecheck"
)

type renderer struct {
	rootDir     string
	outDir      string
	modulePath  string
	importAlias string
	schema      *runtime.LibrarySchema
	index       *goschema.SourceIndex
	comments    *commentReader
}

type category struct {
	Kind       string
	Title      string
	Noun       string
	Dir        string
	Block      string
	TypeSchema map[string]*runtime.TypeSchema
}

func (r renderer) renderAll() error {
	categories := nonemptyCategories(r.categories())
	if err := r.writeReferenceIndex(categories); err != nil {
		return err
	}
	if r.schema.HasConfiguration {
		if err := r.writeConfiguration(); err != nil {
			return err
		}
	}
	for _, cat := range categories {
		if err := r.writeCategory(cat); err != nil {
			return err
		}
	}
	if len(r.schema.Functions) > 0 {
		if err := r.writeFunctions(); err != nil {
			return err
		}
	}
	return nil
}

func (r renderer) categories() []category {
	return []category{
		{
			Kind:       "resource",
			Title:      "Resources",
			Noun:       "resource",
			Dir:        "resources",
			Block:      "resources",
			TypeSchema: r.schema.Resources,
		},
		{
			Kind:       "data-source",
			Title:      "Data sources",
			Noun:       "data source",
			Dir:        "data-sources",
			Block:      "data-sources",
			TypeSchema: r.schema.DataSources,
		},
		{
			Kind:       "action",
			Title:      "Actions",
			Noun:       "action",
			Dir:        "actions",
			Block:      "actions",
			TypeSchema: r.schema.Actions,
		},
	}
}

func nonemptyCategories(categories []category) []category {
	out := make([]category, 0, len(categories))
	for _, cat := range categories {
		if len(cat.TypeSchema) > 0 {
			out = append(out, cat)
		}
	}
	return out
}

func (r renderer) writeReferenceIndex(categories []category) error {
	var b strings.Builder
	b.WriteString("# Reference\n\n")
	b.WriteString("These pages are generated from the library source. ")
	b.WriteString("They list exported kinds and functions, including input fields, ")
	b.WriteString("output fields, constraints, defaults, and sensitive fields when present.\n\n")
	if r.schema.HasConfiguration {
		b.WriteString("- [Configuration](configuration.md)\n")
	}
	for _, cat := range categories {
		fmt.Fprintf(&b, "- [%s](%s/index.md) (%d)\n", cat.Title, cat.Dir, len(cat.TypeSchema))
	}
	if len(r.schema.Functions) > 0 {
		fmt.Fprintf(&b, "- [Functions](functions/index.md) (%d)\n", len(r.schema.Functions))
	}
	return writeFile(filepath.Join(r.outDir, "index.md"), b.String())
}

func (r renderer) writeConfiguration() error {
	var b strings.Builder
	b.WriteString("# Configuration\n\n")
	fmt.Fprintf(&b, "This library uses `library-config('%s')` ", r.modulePath)
	b.WriteString("for per-alias settings. Pass a value to `library-configs:` in ")
	b.WriteString("factory source, usually from a factory input so stack files can choose ")
	b.WriteString("the environment-specific settings.\n")

	configDoc := typeComment{}
	if r.index != nil {
		doc, err := r.comments.typeAt(r.index.ConfigType)
		if err != nil {
			return err
		}
		configDoc = doc
		if doc.Doc != "" {
			fmt.Fprintf(&b, "\n## Description\n\n%s\n", doc.Doc)
		}
	}

	if len(r.schema.ConfigurationFields) > 0 {
		b.WriteString("\n## Fields\n\n")
		writeObjectFieldCards(
			&b,
			r.schema.ConfigurationFields,
			r.schema.ConfigurationDefaults,
			nil,
			configDoc.Fields,
		)
	} else {
		b.WriteString("\nThis library declares an empty configuration schema.\n")
	}
	writeConstraintSection(
		&b,
		"Configuration Constraints",
		"This library declares no configuration constraints.",
		r.schema.ConfigurationConstraints,
	)

	return writeFile(filepath.Join(r.outDir, "configuration.md"), b.String())
}

func (r renderer) writeCategory(cat category) error {
	dir := filepath.Join(r.outDir, cat.Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	names := sortedNames(cat.TypeSchema)
	var summary strings.Builder
	fmt.Fprintf(&summary, "* [%s](index.md)\n", cat.Title)
	for _, name := range names {
		fmt.Fprintf(&summary, "* [%s](%s.md)\n", name, name)
	}
	if err := writeFile(filepath.Join(dir, "SUMMARY.md"), summary.String()); err != nil {
		return err
	}

	if err := r.writeCategoryIndex(cat, names); err != nil {
		return err
	}
	for _, name := range names {
		if err := r.writeKind(cat, name, cat.TypeSchema[name]); err != nil {
			return err
		}
	}
	return nil
}

func (r renderer) writeFunctions() error {
	dir := filepath.Join(r.outDir, "functions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	names := sortedNames(r.schema.Functions)
	var summary strings.Builder
	summary.WriteString("* [Functions](index.md)\n")
	for _, name := range names {
		fmt.Fprintf(&summary, "* [%s](%s.md)\n", name, name)
	}
	if err := writeFile(filepath.Join(dir, "SUMMARY.md"), summary.String()); err != nil {
		return err
	}
	if err := r.writeFunctionsIndex(names); err != nil {
		return err
	}
	for _, name := range names {
		if err := r.writeFunction(name, r.schema.Functions[name]); err != nil {
			return err
		}
	}
	return nil
}

func (r renderer) writeFunctionsIndex(names []string) error {
	var b strings.Builder
	b.WriteString("# Functions\n\n")
	fmt.Fprintf(&b, "This library exports %d %s.\n\n",
		len(names), countWord(len(names), "function", "functions"))
	for _, name := range names {
		sig := functionSignature(r.importAlias+"."+name, r.schema.Functions[name])
		fmt.Fprintf(&b, "- [`%s.%s`](%s.md) — `%s`\n",
			r.importAlias, name, name, sig)
	}
	return writeFile(filepath.Join(r.outDir, "functions", "index.md"), b.String())
}

func (r renderer) writeFunction(name string, sig typecheck.FuncSig) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s.%s function\n\n", r.importAlias, name)
	if src := r.sourceLocation(functionLocation(r.index, name)); src != "" {
		fmt.Fprintf(&b, "Source: `%s`\n\n", src)
	}
	b.WriteString("Signature:\n\n")
	b.WriteString("```\n")
	fmt.Fprintf(&b, "%s\n", functionSignature(r.importAlias+"."+name, sig))
	b.WriteString("```\n")
	return writeFile(filepath.Join(r.outDir, "functions", name+".md"), b.String())
}

func functionLocation(index *goschema.SourceIndex, name string) goschema.GoLocation {
	if index == nil {
		return goschema.GoLocation{}
	}
	return index.Functions[name]
}

func functionSignature(name string, sig typecheck.FuncSig) string {
	params := make([]string, 0, len(sig.Params)+1)
	for _, param := range sig.Params {
		params = append(params, param.String())
	}
	if sig.Variadic != nil {
		params = append(params, "..."+sig.Variadic.String())
	}
	return fmt.Sprintf("%s(%s) %s", name, strings.Join(params, ", "), sig.Result.String())
}

func (r renderer) writeCategoryIndex(cat category, names []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", cat.Title)
	fmt.Fprintf(&b, "This library exports %d %s %s.\n\n",
		len(names), cat.Noun, kindCountWord(len(names)))
	for _, name := range names {
		fmt.Fprintf(&b, "- [`%s.%s`](%s.md)\n", r.importAlias, name, name)
	}
	return writeFile(filepath.Join(r.outDir, cat.Dir, "index.md"), b.String())
}

func (r renderer) writeKind(cat category, name string, ts *runtime.TypeSchema) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s.%s %s\n\n", r.importAlias, name, cat.Noun)

	typeDoc, err := r.kindComment(cat, name)
	if err != nil {
		return err
	}
	if typeDoc.Doc != "" {
		fmt.Fprintf(&b, "## Description\n\n%s\n", typeDoc.Doc)
	}
	if src := r.sourcePath(typeDoc); src != "" {
		fmt.Fprintf(&b, "\nSource: `%s`\n", src)
	}

	b.WriteString("\nExample usage:\n\n")
	b.WriteString("```\n")
	b.WriteString("imports: {\n")
	fmt.Fprintf(&b, "  %s: '%s'\n", r.importAlias, r.modulePath)
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "%s: {\n", cat.Block)
	fmt.Fprintf(&b, "  example: %s.%s {\n", r.importAlias, name)
	if len(ts.Inputs) == 0 {
		b.WriteString("  }\n")
	} else {
		b.WriteString("    # Set input fields here.\n")
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	b.WriteString("```\n")

	r.writeFieldTable(&b, "Inputs", ts.Inputs, inputFieldLocations(r.index, cat.Kind, name),
		true, ts.Defaults, ts.SensitiveInputs, typeDoc.Fields)
	r.writeConstraints(&b, ts.Constraints)
	r.writeFieldTable(&b, "Outputs", ts.Outputs, outputFieldLocations(r.index, cat.Kind, name),
		false, nil, ts.SensitiveOutputs, nil)

	return writeFile(filepath.Join(r.outDir, cat.Dir, name+".md"), b.String())
}

func (r renderer) kindComment(cat category, name string) (typeComment, error) {
	if r.index == nil {
		return typeComment{}, nil
	}
	loc := r.index.InputTypes[cat.Kind][name]
	return r.comments.typeAt(loc)
}

func (r renderer) sourcePath(doc typeComment) string {
	return r.sourceLocation(goschema.GoLocation{Path: doc.Path, Line: doc.Line})
}

func (r renderer) sourceLocation(loc goschema.GoLocation) string {
	if loc.Path == "" {
		return ""
	}
	rel, err := filepath.Rel(r.rootDir, loc.Path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(fmt.Sprintf("%s:%d", rel, loc.Line))
	}
	return ""
}

func writeObjectFieldCards(
	b *strings.Builder,
	fields []typecheck.ObjectField,
	defaults []lang.DefaultSpec,
	sensitive []string,
	comments map[string]string,
) {
	defaultMap := defaultsByField(defaults)
	valueDefaults := valueDefaultsByField(defaults)
	sensitiveSet := stringSet(sensitive)
	b.WriteString("<div class=\"ub-fields\">\n")
	for _, field := range fields {
		_, hasDefault := defaultMap[field.Name]
		valueDefault, hasValueDefault := valueDefaults[field.Name]
		writeFieldCard(b,
			fieldCard{
				Name:            field.Name,
				Type:            displayObjectFieldType(field),
				IncludeRequired: true,
				Required:        objectFieldRequired(field, hasDefault),
				Sensitive:       sensitiveSet[field.Name],
				HasDefault:      hasValueDefault,
				Default:         valueDefault.Value,
				Description:     comments[field.Name],
			},
		)
	}
	b.WriteString("</div>\n")
}

func (r renderer) writeFieldTable(
	b *strings.Builder,
	title string,
	fields map[string]typecheck.Type,
	locations map[string]goschema.GoLocation,
	includeInputColumns bool,
	defaults []lang.DefaultSpec,
	sensitive []string,
	comments map[string]string,
) {
	fmt.Fprintf(b, "\n## %s\n\n", title)
	if len(fields) == 0 {
		fmt.Fprintf(b, "This kind has no %s.\n", strings.ToLower(title))
		return
	}
	defaultMap := defaultsByField(defaults)
	valueDefaults := valueDefaultsByField(defaults)
	sensitiveSet := stringSet(sensitive)
	b.WriteString("<div class=\"ub-fields\">\n")
	for _, name := range sortedFieldNames(fields, locations) {
		_, hasDefault := defaultMap[name]
		valueDefault, hasValueDefault := valueDefaults[name]
		required := false
		if includeInputColumns {
			required = fieldRequired(fields[name], hasDefault)
		}
		writeFieldCard(b,
			fieldCard{
				Name:            name,
				Type:            fields[name],
				IncludeRequired: includeInputColumns,
				Required:        required,
				Sensitive:       sensitiveSet[name],
				HasDefault:      includeInputColumns && hasValueDefault,
				Default:         valueDefault.Value,
				Description:     comments[name],
			},
		)
	}
	b.WriteString("</div>\n")
}

type fieldCard struct {
	Name            string
	Type            typecheck.Type
	IncludeRequired bool
	Required        bool
	Sensitive       bool
	HasDefault      bool
	Default         string
	Description     string
}

func writeFieldCard(b *strings.Builder, field fieldCard) {
	lines := typeLines(field.Type)
	simple := len(lines) == 1 && len(lines[0]) <= 64
	className := "ub-field ub-field--complex"
	if simple {
		className = "ub-field ub-field--simple"
	}
	fmt.Fprintf(b, "<section class=\"%s\">\n", className)
	b.WriteString("<div class=\"ub-field-header\">\n")
	fmt.Fprintf(b, "<p class=\"ub-field-name\"><strong>%s</strong></p>\n",
		html.EscapeString(field.Name),
	)
	if simple {
		fmt.Fprintf(b, "<p class=\"ub-field-type\"><code>%s</code></p>\n",
			html.EscapeString(lines[0]),
		)
	} else {
		fmt.Fprintf(b, "<details class=\"ub-field-type-details\"><summary><code>%s</code></summary>\n",
			html.EscapeString(typeSummary(field.Type)),
		)
		fmt.Fprintf(b, "<pre class=\"ub-field-type-block\"><code>%s</code></pre>\n",
			html.EscapeString(strings.Join(lines, "\n")),
		)
		b.WriteString("</details>\n")
	}
	b.WriteString("</div>\n")
	writeFieldMeta(b, field)
	writeFieldDefault(b, field)
	writeFieldDescription(b, field.Description)
	b.WriteString("</section>\n")
}

func writeFieldMeta(b *strings.Builder, field fieldCard) {
	var parts []string
	if field.IncludeRequired && field.Required {
		parts = append(parts, "<span class=\"ub-badge ub-badge--required\">required</span>")
	}
	if field.Sensitive {
		parts = append(parts, "<span class=\"ub-badge ub-badge--sensitive\">sensitive</span>")
	}
	if len(parts) == 0 {
		return
	}
	fmt.Fprintf(b, "<p class=\"ub-field-meta\">%s</p>\n", strings.Join(parts, " "))
}

func writeFieldDefault(b *strings.Builder, field fieldCard) {
	if !field.HasDefault {
		return
	}
	value := displayDefaultValue(field.Default)
	b.WriteString("<div class=\"ub-field-default\">\n")
	b.WriteString("<span class=\"ub-default-label\">default</span>\n")
	if defaultValueBlock(value) {
		fmt.Fprintf(b, "<pre class=\"ub-default-value-block\"><code>%s</code></pre>\n",
			html.EscapeString(value))
	} else {
		fmt.Fprintf(b, "<code class=\"ub-default-value\">%s</code>\n",
			html.EscapeString(value))
	}
	b.WriteString("</div>\n")
}

func displayDefaultValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return strings.ReplaceAll(value[1:len(value)-1], `\'`, `'`)
	}
	return value
}

func defaultValueBlock(value string) bool {
	return len(value) > 48 || strings.ContainsAny(value, "{}[]\n")
}

func writeFieldDescription(b *strings.Builder, description string) {
	description = strings.TrimSpace(unescapeMarkdownEscapes(description))
	if description == "" {
		return
	}
	for _, paragraph := range strings.Split(description, "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		paragraph = html.EscapeString(paragraph)
		paragraph = strings.ReplaceAll(paragraph, "\n", "<br>\n")
		fmt.Fprintf(b, "<p class=\"ub-field-description\">%s</p>\n", paragraph)
	}
}

func unescapeMarkdownEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && markdownEscapedPunct(s[i+1]) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func markdownEscapedPunct(b byte) bool {
	return strings.ContainsRune(`!"#$%&'()*+,-./:;<=>?@[\]^_`+"`"+`{|}~`, rune(b))
}

func (r renderer) writeConstraints(b *strings.Builder, constraints []lang.ConstraintSpec) {
	writeConstraintSection(
		b,
		"Input Constraints",
		"This kind declares no extra input constraints.",
		constraints,
	)
}

func writeConstraintSection(
	b *strings.Builder,
	title string,
	emptyText string,
	constraints []lang.ConstraintSpec,
) {
	fmt.Fprintf(b, "\n## %s\n\n", title)
	if len(constraints) == 0 {
		fmt.Fprintf(b, "%s\n", emptyText)
		return
	}
	b.WriteString("<div class=\"ub-constraints\">\n")
	for _, group := range constraintGroups(constraints) {
		writeConstraintGroup(b, group)
	}
	b.WriteString("</div>\n")
}

func inputFieldLocations(
	index *goschema.SourceIndex, kind string, name string,
) map[string]goschema.GoLocation {
	if index == nil {
		return nil
	}
	return index.InputFields[kind][name]
}

func outputFieldLocations(
	index *goschema.SourceIndex, kind string, name string,
) map[string]goschema.GoLocation {
	if index == nil {
		return nil
	}
	return index.OutputFields[kind][name]
}

func kindCountWord(n int) string {
	return countWord(n, "kind", "kinds")
}

func countWord(n int, singular string, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func sortedNames[T any](m map[string]T) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedFieldNames(
	fields map[string]typecheck.Type,
	locations map[string]goschema.GoLocation,
) []string {
	names := sortedNames(fields)
	sort.SliceStable(names, func(i, j int) bool {
		left, lok := locations[names[i]]
		right, rok := locations[names[j]]
		if !lok && !rok {
			return names[i] < names[j]
		}
		if !lok {
			return false
		}
		if !rok {
			return true
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		return left.Column < right.Column
	})
	return names
}

func defaultsByField(defaults []lang.DefaultSpec) map[string]lang.DefaultSpec {
	out := map[string]lang.DefaultSpec{}
	for _, def := range defaults {
		field := strings.TrimPrefix(def.Field, "input.")
		if strings.Contains(field, ".") || strings.Contains(field, "[") {
			continue
		}
		out[field] = def
	}
	return out
}

func valueDefaultsByField(defaults []lang.DefaultSpec) map[string]lang.DefaultSpec {
	out := map[string]lang.DefaultSpec{}
	for _, def := range defaults {
		if !hasDefaultValue(def) {
			continue
		}
		field := strings.TrimPrefix(def.Field, "input.")
		if strings.Contains(field, ".") || strings.Contains(field, "[") {
			continue
		}
		out[field] = def
	}
	return out
}

func fieldRequired(typ typecheck.Type, hasDefault bool) bool {
	return !hasDefault && typ.Kind != typecheck.Optional
}

func objectFieldRequired(field typecheck.ObjectField, hasDefault bool) bool {
	return !hasDefault && !field.Optional && !field.Defaulted && field.Type.Kind != typecheck.Optional
}

func displayObjectFieldType(field typecheck.ObjectField) typecheck.Type {
	if field.Optional && field.Type.Kind != typecheck.Optional {
		return typecheck.TOptional(field.Type)
	}
	return field.Type
}

func typeLines(typ typecheck.Type) []string {
	switch typ.Kind {
	case typecheck.List:
		return wrapperTypeLines("list", typ.Elem)
	case typecheck.Map:
		return wrapperTypeLines("map", typ.Elem)
	case typecheck.Optional:
		return wrapperTypeLines("optional", typ.Elem)
	case typecheck.Tuple:
		return typeListLines("tuple", typ.Elems)
	case typecheck.Union:
		return []string{typ.String()}
	case typecheck.Object:
		return objectTypeLines(typ)
	default:
		return []string{typ.String()}
	}
}

func typeSummary(typ typecheck.Type) string {
	switch typ.Kind {
	case typecheck.List:
		return wrapperTypeSummary("list", typ.Elem)
	case typecheck.Map:
		return wrapperTypeSummary("map", typ.Elem)
	case typecheck.Optional:
		return wrapperTypeSummary("optional", typ.Elem)
	case typecheck.Tuple:
		return "tuple"
	case typecheck.Union:
		return "union"
	case typecheck.Object:
		return "object"
	default:
		return typ.String()
	}
}

func wrapperTypeSummary(name string, elem *typecheck.Type) string {
	if elem == nil {
		return name + "(unknown)"
	}
	return name + "(" + typeSummary(*elem) + ")"
}

func wrapperTypeLines(name string, elem *typecheck.Type) []string {
	if elem == nil {
		return []string{name + "(unknown)"}
	}
	lines := typeLines(*elem)
	if len(lines) == 1 {
		return []string{name + "(" + lines[0] + ")"}
	}
	return appendWrappedType(name, lines)
}

func typeListLines(name string, elems []typecheck.Type) []string {
	if len(elems) == 0 {
		return []string{name + "()"}
	}
	parts := make([]string, len(elems))
	allSimple := true
	for i, elem := range elems {
		lines := typeLines(elem)
		if len(lines) != 1 {
			allSimple = false
			break
		}
		parts[i] = lines[0]
	}
	if allSimple {
		return []string{name + "(" + strings.Join(parts, ", ") + ")"}
	}
	var lines []string
	for _, elem := range elems {
		lines = append(lines, typeLines(elem)...)
	}
	return appendWrappedType(name, lines)
}

func objectTypeLines(typ typecheck.Type) []string {
	if len(typ.Fields) == 0 {
		return []string{"object({})"}
	}
	lines := []string{"object({"}
	for _, field := range typ.Fields {
		fieldLines := objectFieldTypeLines(field)
		lines = append(lines, "  "+field.Name+": "+fieldLines[0])
		for _, line := range fieldLines[1:] {
			lines = append(lines, "  "+line)
		}
	}
	lines = append(lines, "})")
	if typ.Open {
		return appendWrappedType("open", lines)
	}
	return lines
}

func objectFieldTypeLines(field typecheck.ObjectField) []string {
	if field.Optional {
		return typeLines(typecheck.TOptional(field.Type))
	}
	return typeLines(field.Type)
}

func appendWrappedType(name string, inner []string) []string {
	lines := []string{name + "("}
	for _, line := range inner {
		lines = append(lines, "  "+line)
	}
	lines = append(lines, ")")
	return lines
}

func hasDefaultValue(def lang.DefaultSpec) bool {
	return !def.Optional && def.Value != ""
}

type constraintGroup struct {
	key         string
	title       string
	constraints []lang.ConstraintSpec
}

func constraintGroups(constraints []lang.ConstraintSpec) []constraintGroup {
	indexes := map[string]int{}
	groups := make([]constraintGroup, 0, len(constraints))
	for _, constraint := range constraints {
		key := constraintGroupKey(constraint)
		idx, ok := indexes[key]
		if !ok {
			idx = len(groups)
			indexes[key] = idx
			groups = append(groups, constraintGroup{
				key:   key,
				title: constraintGroupTitle(key),
			})
		}
		groups[idx].constraints = append(groups[idx].constraints, constraint)
	}
	return groups
}

func writeConstraintGroup(b *strings.Builder, group constraintGroup) {
	fmt.Fprintf(b, "<section class=\"ub-constraint-group\" data-constraint-group=\"%s\">\n",
		html.EscapeString(group.key),
	)
	fmt.Fprintf(b, "<h3 class=\"ub-constraint-group-title\">%s</h3>\n",
		html.EscapeString(group.title),
	)
	for _, constraint := range group.constraints {
		writeConstraintCard(b, constraint)
	}
	b.WriteString("</section>\n")
}

func writeConstraintCard(b *strings.Builder, spec lang.ConstraintSpec) {
	b.WriteString("<div class=\"ub-constraint\">\n")
	fmt.Fprintf(b, "<p class=\"ub-constraint-summary\">%s</p>\n", constraintSummaryHTML(spec))
	if details := constraintDetailsHTML(spec); details != "" {
		b.WriteString(details)
	}
	b.WriteString("</div>\n")
}

func constraintSummaryHTML(spec lang.ConstraintSpec) string {
	if strings.TrimSpace(spec.Message) != "" {
		return constraintMessageHTML(
			sentenceText(spec.Message),
			constraintSummaryFieldNames(spec),
			constraintStringLiterals(spec),
		)
	}
	if len(spec.Fields) > 0 {
		return fmt.Sprintf("%s %s.",
			constraintSummaryTitle(spec.Kind),
			fieldListHTML(spec.Fields, constraintFieldConjunction(spec.Kind)),
		)
	}
	return html.EscapeString(sentenceText(constraintTitle(spec.Kind)))
}

func constraintSummaryTitle(kind string) string {
	switch kind {
	case "required-together", "required-with":
		return "Required together:"
	case "forbidden-with":
		return "Forbidden together:"
	default:
		return constraintTitle(kind)
	}
}

func constraintFieldConjunction(kind string) string {
	switch kind {
	case "at-least-one-of", "at-most-one-of", "exactly-one-of":
		return "or"
	default:
		return "and"
	}
}

func constraintDetailsHTML(spec lang.ConstraintSpec) string {
	sections := []struct {
		title string
		body  string
	}{}
	if spec.ForEach != "" || len(spec.ForEachLevels) > 0 {
		sections = append(sections, struct {
			title string
			body  string
		}{"For each", constraintForEachHTML(spec)})
	}
	if when := expressionHTML(spec.When); when != "" && strings.TrimSpace(spec.When) != "true" {
		sections = append(sections, struct {
			title string
			body  string
		}{"When", when})
	}
	if require := expressionHTML(spec.Require); require != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"Require", require})
	}
	if len(sections) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<details class=\"ub-constraint-logic\">\n")
	b.WriteString("<summary>Rule logic</summary>\n")
	b.WriteString("<dl>\n")
	for _, section := range sections {
		fmt.Fprintf(&b, "<dt>%s</dt>\n", html.EscapeString(section.title))
		fmt.Fprintf(&b, "<dd>%s</dd>\n", section.body)
	}
	b.WriteString("</dl>\n")
	b.WriteString("</details>\n")
	return b.String()
}

func constraintTitle(kind string) string {
	switch kind {
	case "at-most-one-of":
		return "At most one of"
	case "at-least-one-of":
		return "At least one of"
	case "exactly-one-of":
		return "Exactly one of"
	case "required-together":
		return "Required together"
	case "required-with":
		return "Required with"
	case "forbidden-with":
		return "Forbidden with"
	case "predicate":
		return "Conditional requirement"
	default:
		return kind
	}
}

func constraintForEachHTML(spec lang.ConstraintSpec) string {
	if spec.ForEach != "" {
		return codeHTML(spec.ForEach)
	}
	parts := make([]string, 0, len(spec.ForEachLevels))
	for _, level := range spec.ForEachLevels {
		parts = append(parts, fmt.Sprintf("%s in %s",
			codeHTML(forEachLevelName(level.Name)), codeHTML(level.In)))
	}
	return strings.Join(parts, "<br>\n")
}

func expressionHTML(src string) string {
	rows := expressionRows(src)
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		expr := row.Expr
		if row.Connector != "" {
			expr = row.Connector + " " + expr
		}
		lines = append(lines, codeHTML(expr))
	}
	return strings.Join(lines, "<br>\n")
}

func codeHTML(s string) string {
	return "<code>" + html.EscapeString(s) + "</code>"
}

func fieldListHTML(values []string, conjunction string) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strongHTML(displayFieldRef(value))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	if len(parts) == 2 {
		return parts[0] + " " + conjunction + " " + parts[1]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", " + conjunction + " " + parts[len(parts)-1]
}

func strongHTML(s string) string {
	return "<strong>" + html.EscapeString(s) + "</strong>"
}

func displayFieldRef(ref string) string {
	return strings.TrimPrefix(strings.TrimSpace(ref), "input.")
}

func constraintSummaryFieldNames(spec lang.ConstraintSpec) []string {
	seen := map[string]bool{}
	add := func(field string) {
		field = constraintFieldSegment(field)
		if field == "" || seen[field] {
			return
		}
		seen[field] = true
	}
	add(spec.ForEach)
	for _, level := range spec.ForEachLevels {
		add(level.In)
	}
	for _, field := range spec.Fields {
		add(field)
	}
	for _, field := range inputFields(spec.When) {
		add(field)
	}
	for _, field := range inputFields(spec.Require) {
		add(field)
	}
	return sortedLongestFirst(seen)
}

func constraintStringLiterals(spec lang.ConstraintSpec) []string {
	seen := map[string]bool{}
	for _, literal := range stringLiterals(spec.When) {
		seen[literal] = true
	}
	for _, literal := range stringLiterals(spec.Require) {
		seen[literal] = true
	}
	return sortedLongestFirst(seen)
}

func sortedLongestFirst(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

func stringLiterals(src string) []string {
	var out []string
	for i := 0; i < len(src); i++ {
		if src[i] != '\'' {
			continue
		}
		literal, end, ok := readSingleQuoted(src, i)
		if !ok {
			continue
		}
		out = append(out, literal)
		i = end
	}
	return out
}

func readSingleQuoted(src string, start int) (string, int, bool) {
	var b strings.Builder
	for i := start + 1; i < len(src); i++ {
		if src[i] == '\\' && i+1 < len(src) {
			i++
			b.WriteByte(src[i])
			continue
		}
		if src[i] == '\'' {
			return b.String(), i, true
		}
		b.WriteByte(src[i])
	}
	return "", start, false
}

func constraintMessageHTML(text string, fields []string, literals []string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if matched := matchSummaryToken(text, i, fields, fieldRefBoundary); matched != "" {
			b.WriteString(strongHTML(matched))
			i += len(matched)
			continue
		}
		if matched := matchSummaryToken(text, i, literals, literalBoundary); matched != "" {
			b.WriteString(codeHTML(matched))
			i += len(matched)
			continue
		}
		b.WriteString(html.EscapeString(text[i : i+1]))
		i++
	}
	return b.String()
}

func matchSummaryToken(
	text string,
	start int,
	values []string,
	boundary func(string, int, int) bool,
) string {
	for _, value := range values {
		if !strings.HasPrefix(text[start:], value) {
			continue
		}
		end := start + len(value)
		if boundary(text, start, end) {
			return value
		}
	}
	return ""
}

func fieldRefBoundary(text string, start, end int) bool {
	return (start == 0 || !fieldNameByte(text[start-1])) &&
		(end == len(text) || !fieldNameByte(text[end]))
}

func literalBoundary(text string, start, end int) bool {
	return (start == 0 || !literalByte(text[start-1])) &&
		(end == len(text) || !literalByte(text[end]))
}

func fieldNameByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '-' || b == '_'
}

func literalByte(b byte) bool {
	return fieldNameByte(b) || b == '@'
}

func sentenceText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.ContainsAny(s[len(s)-1:], ".!?") {
		s += "."
	}
	return s
}

func forEachLevelName(name string) string {
	if strings.HasPrefix(name, "@") {
		return name
	}
	return "@" + name
}

var inputRefPattern = regexp.MustCompile(`\binput\.([A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*)`)

func constraintFieldSegment(src string) string {
	if field := firstInputField(src); field != "" {
		return field
	}
	return fieldSegment(src)
}

func constraintGroupKey(spec lang.ConstraintSpec) string {
	if field := constraintFieldSegment(spec.ForEach); field != "" {
		return field
	}
	if len(spec.ForEachLevels) > 0 {
		if field := constraintFieldSegment(spec.ForEachLevels[0].In); field != "" {
			return field
		}
	}
	if strings.TrimSpace(spec.When) != "true" {
		if field := firstInputField(spec.When); field != "" {
			return field
		}
	}
	if field := fieldGroupFromFields(spec.Fields); field != "" {
		return field
	}
	if field := firstInputField(spec.Require); field != "" {
		return field
	}
	return "general"
}

func fieldGroupFromFields(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	first := fieldSegment(fields[0])
	if first == "" {
		return "field-combinations"
	}
	for _, field := range fields[1:] {
		if fieldSegment(field) != first {
			return "field-combinations"
		}
	}
	return first
}

func inputFields(src string) []string {
	matches := inputRefPattern.FindAllStringSubmatch(src, -1)
	fields := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		fields = append(fields, "input."+match[1])
	}
	return fields
}

func firstInputField(src string) string {
	fields := inputFields(src)
	if len(fields) == 0 {
		return ""
	}
	return fieldSegment(fields[0])
}

func fieldSegment(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "input.")
	if ref == "" || strings.HasPrefix(ref, "@") {
		return ""
	}
	ref = strings.Split(ref, ".")[0]
	ref = strings.Split(ref, "[")[0]
	return ref
}

func constraintGroupTitle(key string) string {
	switch key {
	case "field-combinations":
		return "Field combinations"
	case "general", "":
		return "General rules"
	default:
		return fieldTitle(key) + " rules"
	}
}

func fieldTitle(field string) string {
	field = strings.ReplaceAll(field, "_", "-")
	parts := strings.Split(field, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 && part[0] >= 'a' && part[0] <= 'z' {
			parts[i] = string(part[0]-'a'+'A') + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

type expressionRow struct {
	Connector string
	Expr      string
}

func expressionRows(src string) []expressionRow {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil
	}
	expr := src
	if inner, ok := unwrapOuterParens(src); ok {
		expr = inner
	}
	if parts := splitTopLevel(expr, "||"); len(parts) > 1 {
		return expressionPartRows(parts, "||")
	}
	if parts := splitTopLevel(expr, "&&"); len(parts) > 1 {
		return expressionPartRows(parts, "&&")
	}
	return []expressionRow{{Expr: unwrapSimpleOuterParens(src)}}
}

func expressionPartRows(parts []string, connector string) []expressionRow {
	rows := make([]expressionRow, 0, len(parts))
	for i, part := range parts {
		row := expressionRow{Expr: unwrapSimpleOuterParens(strings.TrimSpace(part))}
		if i > 0 {
			row.Connector = connector
		}
		rows = append(rows, row)
	}
	return rows
}

func unwrapSimpleOuterParens(src string) string {
	inner, ok := unwrapOuterParens(src)
	if !ok {
		return src
	}
	if len(splitTopLevel(inner, "||")) > 1 || len(splitTopLevel(inner, "&&")) > 1 {
		return src
	}
	return inner
}

func splitTopLevel(src, op string) []string {
	var parts []string
	depth := 0
	start := 0
	inString := false
	escaped := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '\'' {
				inString = false
			}
			continue
		}
		switch c {
		case '\'':
			inString = true
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && strings.HasPrefix(src[i:], op) {
				parts = append(parts, strings.TrimSpace(src[start:i]))
				i += len(op) - 1
				start = i + 1
			}
		}
	}
	if len(parts) == 0 {
		return nil
	}
	parts = append(parts, strings.TrimSpace(src[start:]))
	return parts
}

func unwrapOuterParens(src string) (string, bool) {
	src = strings.TrimSpace(src)
	if len(src) < 2 || src[0] != '(' || src[len(src)-1] != ')' {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '\'' {
				inString = false
			}
			continue
		}
		switch c {
		case '\'':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(src)-1 {
				return "", false
			}
		}
	}
	return strings.TrimSpace(src[1 : len(src)-1]), depth == 0
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
