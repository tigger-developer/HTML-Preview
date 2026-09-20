---
title: Annotation target fixtures
version: 1
last-updated: 2026-09-20
---

# Annotation target fixtures

These synthetic documents exercise the rendered HTML that the annotation
browser code consumes. They contain no personal document content.

`TestHTTPAnnotationTargetContract` serves each document through the real HTTP
service, checks annotation metadata, and starts target selection from visible
text and inline elements. The nearest eligible block must carry its own verified
`data-hp-annotation-block` attribute. A marker on some other element is insufficient.
The test also creates a footnote through HTTP and checks the refreshed document.

- `blocks.org` covers headings, formatting, links, task lists, descriptions,
  code, tables, quote/verse containers and nested definition lists. Empty `::`
  boundaries must not make neighbouring blocks lose their annotation targets.
- `blocks.md` covers the corresponding supported Markdown targets. Its definition
  list remains a reading sample: the existing source-boundary scanner excludes
  colon-prefixed Markdown description lines from annotation creation.

Browser event handling, focus and layout still require user validation. These
tests check the served HTML contract and HTTP writes; they do not execute a browser.
