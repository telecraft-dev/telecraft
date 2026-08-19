---
title: Reference
description: Lookup documentation for the Telecraft command line and every authored file format.
order: 1
---

# Reference

The reference describes what exists: every binary and subcommand, every field
of every authored file format, the layout of an estate repository, and the
rules each loader applies. It's written for lookup. When you want to be walked
through a task, read [the guides](../guides/); when you want the model behind
the vocabulary, read [the concepts](../concepts/).

Every page here is derived from the code that ships. Field names, defaults and
validation rules match the loaders, not the design corpus. Where a decision
record and the code disagree, the code is what these pages document.

## Vocabulary

The [glossary](../glossary.md) is the vocabulary reference. Terms such as
Tier, Service Class, Blueprint, Component, Allow-list, Grant and Exemption are
used here exactly as the glossary defines them, and are not varied for
readability.

## Pages

Command line
: [Command line reference](cli.md) covers `telecraft` and its subcommands
  (`observe`, `check`, `palette`, `render`, `serve`, `snapshot`, `delivery`,
  `passwd`), plus `catalogue-import` and `blueprint-check`: purpose, flags,
  defaults and exit codes.

Repository layout
: [Estate layout](estate-layout.md) covers the estate repository: the root
  files, the team directories, where each authored object lives, the generated
  `rendered/` tree and `CODEOWNERS`, and which paths humans write.

Authored formats
: [Blueprints](blueprints.md), [Tiers](tiers.md),
  [Requirements](requirements.md), [Allow-lists and Grants](allow-lists.md)
  and [Exemptions](exemptions.md) each document one file format: every field,
  its type, whether it's required, its default, and what the loader rejects.

Catalogue
: [Catalogue](catalogue.md) covers the versioned inventory of otelcol
  component types: how it's sourced from upstream `metadata.yaml`, the
  `(class, type)` key, artefact versioning, and importing with
  `catalogue-import`.

## Conventions on these pages

- Field tables give the authored YAML or JSON key, not the Go field name.
- A field marked required is one whose absence fails the load.
- "Load error" means the loader refuses the whole file or tree and returns
  nothing. "Finding" means the load succeeds and the problem is reported to an
  owner instead.
- Placeholders in commands use upper snake case, such as `ESTATE_DIR`, and are
  explained where they first appear.
