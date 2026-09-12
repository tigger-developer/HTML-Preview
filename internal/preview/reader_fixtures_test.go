// ABOUTME: Accounts for every installed Pandoc reader using synthetic, licensable documents.
// ABOUTME: Generates containers through real Pandoc and asserts their published content and media.
package preview

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
)

// All fixture text is project-owned synthetic data under the project licence.
// Writer-derived fixtures use Pandoc's installed writer and its packaged defaults;
// no downloaded/user document or editor configuration enters the corpus.
func readerFixture(t *testing.T, root, reader string, media bool) []byte {
	t.Helper()
	plain := map[string]string{
		"bibtex":     "@book{sample,title={Reader fixture},author={Example, Alice},year={2026}}",
		"biblatex":   "@book{sample,title={Reader fixture},author={Example, Alice},year={2026}}",
		"csljson":    `[{"id":"sample","type":"book","title":"Reader fixture","author":[{"family":"Example","given":"Alice"}]}]`,
		"endnotexml": `<?xml version="1.0"?><xml><records><record><ref-type name="Book">6</ref-type><titles><title>Reader fixture</title></titles></record></records></xml>`,
		"ris":        "TY  - BOOK\nTI  - Reader fixture\nAU  - Example, Alice\nER  - \n",
		"bits":       `<book-part-wrapper><book-part><body><sec><title>Reader fixture</title><p>Reader body</p></sec></body></book-part></book-part-wrapper>`,
		"creole":     "= Reader fixture =\n",
		"csv":        "Title,Value\nReader fixture,42\n",
		"tsv":        "Title\tValue\nReader fixture\t42\n",
		"mdoc":       ".Dd September 12, 2026\n.Dt FIXTURE 1\n.Os\n.Sh NAME\n.Nm Reader fixture\n.Nd sample document\n",
		"pod":        "=head1 Reader fixture\n\nReader body.\n\n=cut\n",
		"t2t":        "Reader fixture\nAlice Example\n2026\n\nReader fixture\n",
		"tikiwiki":   "! Reader fixture\n",
		"twiki":      "---+ Reader fixture\n",
		"vimwiki":    "= Reader fixture =\n",
	}
	if input, ok := plain[reader]; ok {
		return []byte(input)
	}
	if reader == "xlsx" {
		return spreadsheetFixture(t)
	}
	writers := strings.Fields("asciidoc commonmark commonmark_x djot docbook docx dokuwiki epub fb2 gfm haddock html ipynb jats jira json latex man markdown markdown_github markdown_mmd markdown_phpextra markdown_strict mediawiki muse native odt opml org pptx rst rtf textile typst xml")
	found := false
	for _, writer := range writers {
		if reader == writer {
			found = true
		}
	}
	if !found {
		t.Fatalf("installed reader %q has no fixture; extend the catalogue", reader)
	}
	text := "# Reader fixture\n\nReader body.\n\n[next](next.md)\n"
	if media {
		text += "\n![fixture image](data:image/png;base64," + base64.StdEncoding.EncodeToString(rasterFixture(t)) + ")\n"
	}
	host := NativeHost()
	pandoc, err := host.LookPath("pandoc")
	if err != nil {
		t.Fatal(err)
	}
	output, err := host.Execute(t.Context(), Command{Path: pandoc, Args: []string{"--sandbox", "--data-dir=" + root, "--from=markdown", "--to=" + reader, "--standalone"}, Dir: root, Input: []byte(text), Limit: 4 << 20})
	if err != nil {
		t.Fatalf("generate %s fixture: %v", reader, err)
	}
	return output
}

func rasterFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 30, G: 70, B: 110, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func zipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for name, body := range files {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func spreadsheetFixture(t *testing.T) []byte {
	return zipFixture(t, map[string]string{
		"[Content_Types].xml":        `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Fixture" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Reader fixture</t></is></c></row></sheetData></worksheet>`,
	})
}

func TestRT008_1_EveryInstalledReader(t *testing.T) {
	root := t.TempDir()
	listing := run(t, root, nil, "--list-input-formats")
	success(t, listing, 0)
	for _, reader := range strings.Fields(listing.stdout) {
		t.Run(reader, func(t *testing.T) {
			input := readerFixture(t, root, reader, false)
			path := source(t, root, "fixture."+reader, string(input))
			r := run(t, root, nil, "--from="+reader, path)
			success(t, r, 1)
			if !strings.Contains(textOf(nodes(r.pages[0], "body")[0]), "Reader fixture") {
				t.Fatalf("%s lost body/metadata", reader)
			}
		})
	}
}

func TestRT008_7_ContainerMedia(t *testing.T) {
	for _, reader := range []string{"docx", "odt", "epub"} {
		t.Run(reader, func(t *testing.T) {
			root := t.TempDir()
			path := source(t, root, "document."+reader, string(readerFixture(t, root, reader, true)))
			next := source(t, root, "next.md", "Next document")
			r := run(t, t.TempDir(), nil, path)
			success(t, r, 1)
			if attr(documentNode(t, r.pages[0], "hp-source"), "data-hp-source") != path {
				t.Fatal("container lost original source identity")
			}
			images := nodes(r.pages[0], "img")
			if len(images) != 1 {
				t.Fatalf("embedded raster count=%d", len(images))
			}
			encoded, ok := strings.CutPrefix(attr(images[0], "src"), "data:image/png;base64,")
			if !ok {
				t.Fatalf("container media not admitted: %q", attr(images[0], "src"))
			}
			imageData, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || !bytes.Equal(imageData, rasterFixture(t)) {
				t.Fatal("container raster bytes changed")
			}
			for _, a := range nodes(r.pages[0], "a") {
				if textOf(a) == "next" && !strings.HasSuffix(attr(a, "href"), filepath.Base(next)) {
					t.Fatal("container link lost original context")
				}
			}
		})
	}
}

func TestRT008_8_ArchiveAdmission(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "a/../../escape", "C:/absolute", "a\\..\\escape"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			input := zipFixture(t, map[string]string{name: "escaped", "word/document.xml": "<document/>"})
			r := run(t, root, nil, source(t, root, "bad.docx", string(input)))
			if r.code != 1 || len(r.opens) != 0 || !strings.Contains(r.stderr, "archive") {
				t.Fatalf("archive path not rejected before conversion: %s", r.stderr)
			}
		})
	}
	root := t.TempDir()
	input := zipFixture(t, map[string]string{"same": "one", "./same": "two"})
	r := run(t, root, nil, source(t, root, "duplicate.docx", string(input)))
	if r.code != 1 || !strings.Contains(r.stderr, "duplicate") {
		t.Fatal("duplicate normalized names not rejected")
	}
}
