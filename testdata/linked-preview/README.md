---
title: Linked preview fixture
version: 1
last-updated: 2026-09-20
---

# Linked preview fixture

Open `index.org` through a running HTML Preview service whose roots include
this directory. It links to Org, Markdown, DOCX, plain text and Go source.
Each format has distinctive content for visual inspection. Org, Markdown and
DOCX also link to one another and back to the index.

The fixture supports the cross-format browser checks in
[the service specification](../../specs/006-local-preview-service/spec.org).
Successful HTTP requests establish conversion and navigation availability;
visual presentation and browser interaction require human review.

To regenerate the synthetic DOCX from the repository root:

```sh
pandoc --from=markdown --to=docx testdata/linked-preview/document-source.md --output=testdata/linked-preview/document.docx
```

No fixture code should be executed during preview. Org and Markdown service
pages can also expose annotation controls. Use disposable copies for write tests
so the retained reading fixtures are not changed. Dedicated annotation cases
are documented in [the annotation fixture guide](../annotation-targets/README.md).

## Document changes

- Version 1: clarify annotation availability and disposable-copy use.
