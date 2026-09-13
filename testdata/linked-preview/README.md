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

No fixture code should be executed during preview. Annotation controls are
outside this reading fixture's current scope; their delivery is tracked in
[the annotation specification](../../specs/007-service-annotations/spec.org).
