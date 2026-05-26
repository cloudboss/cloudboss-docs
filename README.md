# cloudboss-docs

Shared documentation tooling for cloudboss project docs. It carries the look in one place so every project's docs match, and publishes them the same way: versioned, with a version dropdown, served from GitHub Pages.

The tooling uses [Material for MkDocs](https://squidfunk.github.io/mkdocs-material/) and [mike](https://github.com/jimporter/mike). Search is Material's builtin offline index, so there is no external search service to depend on.

## Contents

- `cloudboss_docs/theme/`: A MkDocs theme named `cloudboss` that extends `material` and bakes in the look: Cantarell, the cloudboss palette, the header logo size, and the callout treatment. The CSS is injected by the theme itself, so a project needs no styling of its own.
- `pyproject.toml`: Packages the theme and declares its direct dependencies.
- `requirements.txt`: Every dependency, direct and transitive, pinned to an exact version for reproducible builds.
- `.github/workflows/docs.yml`: A reusable workflow a project's release job calls to build and publish.

## Adding docs to a project

Install the pinned toolchain and the theme straight from git -- the lockfile is fetched by URL, and `--no-deps` on the theme keeps the pinned versions intact:

```bash
pip install -r https://raw.githubusercontent.com/cloudboss/cloudboss-docs/main/requirements.txt
pip install --no-deps "git+https://github.com/cloudboss/cloudboss-docs"
```

Give the project a `mkdocs.yml` that selects the theme. A theme can't set top level config, so the Markdown extensions, plugins, and the `mike` version provider live here.

Copy this block verbatim and fill in the project's own `site_name`, `site_url`, `repo_url`, `nav`, and logo:

```yaml
site_name: myproject
site_url: https://www.cloudboss.co/myproject/    # The /<repo>/ path is the GitHub Pages path.
repo_url: https://github.com/cloudboss/myproject
repo_name: cloudboss/myproject
copyright: Copyright (c) 2026 cloudboss

theme:
  name: cloudboss
  logo: assets/logo.svg    # A transparent-background SVG under docs/assets/.
  favicon: assets/logo.svg

markdown_extensions:
  - admonition
  - tables
  - toc:
      permalink: true
  - pymdownx.highlight:
      anchor_linenums: true
  - pymdownx.inlinehilite
  - pymdownx.superfences

plugins:
  - search

extra:
  version:
    provider: mike
    default: latest

nav:
  - Introduction:
      - Overview: index.md
```

Put handwritten prose under the project's `docs/`. Reference pages can be generated from the project's own code (see [below](#generating-reference-from-code)) and stored alongside.

## Local preview

From the project directory, install the pinned toolchain and serve:

```bash
pip install -r https://raw.githubusercontent.com/cloudboss/cloudboss-docs/main/requirements.txt
pip install --no-deps "git+https://github.com/cloudboss/cloudboss-docs"
mkdocs serve
```

`mkdocs serve` watches the project's `docs/` and `mkdocs.yml`, but not the installed theme package, so edits to the shared theme need a server restart to show up.

## Generating reference from code

Handwritten guides live in `docs/`. Reference pages, like CLI options or API references, are generated from the program itself, so they never drift from the build that produced them. For every project, it should expose one command that writes Markdown into its docs tree, and the build runs that for each release.

### The generator command

The generator is the project's own program, in whatever language and tooling is suitable for the project. Its job is to write Markdown into the docs tree, usually under `docs/reference/`. How it produces that Markdown is up to the project.

As one example, a Go project can walk its own command tree for a CLI reference and read its package comments through `go/doc` for an API reference. A project in another language does the equivalent with its own tools; nothing here assumes Go or any particular CLI library.

### Running it before the build

The reusable GitHub workflow runs whatever shell command you pass as its `generate` input before the site is built, so use whatever the project already has: a Make target, a shell script, a language's own build runner. Run that same command locally before `mkdocs serve` so the generated pages are present.

```yaml
with:
  version: ${{ github.ref_name }}
  generate: make docs-gen
```

### Nav for the generated pages

List them in `nav:` by hand, or let them build themselves: have the generator also write a `reference/SUMMARY.md`, add the `literate-nav` plugin, and the section assembles from that. Generated output under `docs/reference/` can be git-ignored, since the build recreates it.

### Keeping generated files off disk entirely

If you would rather nothing generated ever lives in the tree, use the `mkdocs-gen-files` plugin: a small Python hook shells out to the program (for example `myapp schema --json`) and writes the pages virtually at build time. That needs `mkdocs-gen-files` added to the cloudboss-docs dependencies -- not included yet (`mkdocs-literate-nav` already is).

## Publishing on release

The site publishes to `https://www.cloudboss.co/<repo>/` from the project repo's `gh-pages` branch. Call the reusable workflow from the project's release job:

```yaml
jobs:
  docs:
    uses: cloudboss/cloudboss-docs/.github/workflows/docs.yml@main
    with:
      version: ${{ github.ref_name }}
      generate: make docs-gen
```

`mike` adds the tag to the version dropdown and moves `latest` onto it. The site root redirects to `latest`.
