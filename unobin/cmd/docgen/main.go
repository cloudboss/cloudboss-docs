// Command docgen emits Unobin library reference Markdown.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cloudboss/cloudboss-docs/unobin/internal/docgen"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("docgen", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "Unobin library module root")
	out := flags.String("out", "docs/reference", "directory to write reference Markdown into")
	flags.StringVar(out, "o", "docs/reference", "directory to write reference Markdown into")
	modulePath := flags.String("module", "", "library module path override")
	alias := flags.String("alias", "", "library import alias override")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return usage()
	}
	return docgen.Generate(docgen.Options{
		RootDir:     *root,
		OutDir:      *out,
		ModulePath:  *modulePath,
		ImportAlias: *alias,
	})
}

func usage() error {
	return fmt.Errorf("usage: docgen [--root .] [--out docs/reference] [--module path] [--alias name]")
}
