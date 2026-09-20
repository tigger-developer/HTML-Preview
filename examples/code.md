# Code and navigation examples

This synthetic document accompanies [the Org work ledger](work.org).
Inline `htmlpreview` and `literal <text> & spaces` offer direct copying.
Use the adjacent copy glyph when code is part of a link:
[`work.org`](work.org) and [two values: `first` and `second`](work.org).

## Highlighted source

```go
package main

func main() {
	println("Taḋg: á é í ó ú <&>")
}
```

```lua
local message = "A local preview"
print(message)
```

```bash
for item in "one" "two"; do
  printf '%s\n' "$item"
done
```

```json
{"toc": true, "tocDepth": 3, "standalone": true}
```

### Literal code and selection

```not-a-language
The language is unknown; this text stays plain.
	An actual tab begins this line.
</script><script>window.sourceExecuted = true</script>
```

The script-looking line above is code text. It must never execute.

    Indented Markdown code is also a copy target.
    Its indentation follows the native Markdown parser's code value.

An empty code block has no enabled copy action:

```
```

#### A fourth-level heading

The default contents depth omits this entry. Setting
`HTMLPREVIEW_TOC_DEPTH=4` includes it.

## A repeated heading {#repeat}

First occurrence. Its table-of-contents entry must target this section.

## A repeated heading {#repeat}

Second occurrence. It receives a distinct final destination.

## A long line

The following retained HTML examples demonstrate omission in Markdown. Inline
tags disappear, leaving their text without a code-copy control; complete HTML
blocks below disappear. The preview and CLI show one omission warning.

Raw semantic <code>inline HTML code</code> becomes ordinary text.

<pre><code>Raw HTML block &lt;text&gt; &amp; spaces
</code></pre>

This older HTML copy example contains two code values. Native HTML previews
retain that behaviour; Markdown omits the whole block:

<pre><code>first value</code> / <code>second value</code></pre>

This older whitespace-only HTML example is also omitted from Markdown:

<pre><code> 	 
</code></pre>

```text
This line deliberately stays on one line so horizontal code scrolling can be inspected at a narrow viewport without pushing the folding bar or copy control over the document text. Taḋg <&> 0123456789.
```

Selecting text by dragging does not copy it. The first click of a double-click
may copy before the browser selects the word. Clipboard refusal offers a
readonly manual-copy field. No-JavaScript reading keeps all content open;
printing includes all content and hides the controls.

## Document changes

- 20 September 2026: native Markdown code parsing and intentional HTML omission
  replace the former Pandoc and raw-HTML copying expectations.
