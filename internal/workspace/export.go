package workspace

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
)

type richNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Content []richNode     `json:"content"`
	Attrs   map[string]any `json:"attrs"`
	Marks   []struct {
		Type  string         `json:"type"`
		Attrs map[string]any `json:"attrs"`
	} `json:"marks"`
}

func xmlText(s string) string {
	var b bytes.Buffer
	s = strings.Map(func(r rune) rune {
		if r == 9 || r == 10 || r == 13 || (r >= 32 && r != 0xFFFE && r != 0xFFFF) {
			return r
		}
		return -1
	}, s)
	xml.EscapeText(&b, []byte(s))
	return b.String()
}
func richBlocks(c Chapter) []richNode {
	var root richNode
	if json.Unmarshal(c.Content, &root) != nil || root.Type != "doc" {
		json.Unmarshal(EmptyDoc(c.Text), &root)
	}
	var blocks []richNode
	var walk func(richNode)
	walk = func(n richNode) {
		if n.Type == "paragraph" || n.Type == "heading" {
			blocks = append(blocks, n)
			return
		}
		for _, child := range n.Content {
			walk(child)
		}
	}
	walk(root)
	return blocks
}
func orderedChapters(p Project) []Chapter {
	var out []Chapter
	for _, v := range p.Volumes {
		for _, c := range p.Chapters {
			if c.VolumeID == v.ID {
				out = append(out, c)
			}
		}
	}
	return out
}
func wordRuns(n richNode) string {
	if n.Type == "hardBreak" {
		return "<w:r><w:br/></w:r>"
	}
	if n.Type == "text" {
		var props strings.Builder
		for _, m := range n.Marks {
			switch m.Type {
			case "bold":
				props.WriteString("<w:b/>")
			case "italic":
				props.WriteString("<w:i/>")
			case "underline":
				props.WriteString(`<w:u w:val="single"/>`)
			case "strike":
				props.WriteString("<w:strike/>")
			case "textStyle":
				if f, ok := m.Attrs["fontFamily"].(string); ok {
					fmt.Fprintf(&props, `<w:rFonts w:ascii="%s" w:hAnsi="%s" w:eastAsia="%s"/>`, xmlText(f), xmlText(f), xmlText(f))
				}
			}
		}
		return "<w:r><w:rPr>" + props.String() + `</w:rPr><w:t xml:space="preserve">` + xmlText(n.Text) + "</w:t></w:r>"
	}
	var b strings.Builder
	for _, child := range n.Content {
		b.WriteString(wordRuns(child))
	}
	return b.String()
}
func wordHeading(s, style string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + style + `"/></w:pPr><w:r><w:t>` + xmlText(s) + `</w:t></w:r></w:p>`
}
func zipPart(z *zip.Writer, name, body string, store bool) error {
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	if store {
		h.Method = zip.Store
	}
	w, err := z.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(body))
	return err
}
func ExportDOCX(p Project) ([]byte, error) {
	var body strings.Builder
	body.WriteString(wordHeading(p.Name, "Title"))
	for _, v := range p.Volumes {
		body.WriteString(wordHeading(v.Name, "Heading1"))
		for _, c := range p.Chapters {
			if c.VolumeID != v.ID {
				continue
			}
			body.WriteString(wordHeading(c.Title, "Heading2"))
			for _, n := range richBlocks(c) {
				props := ""
				if n.Type == "heading" {
					props = `<w:pStyle w:val="Heading2"/>`
				}
				body.WriteString("<w:p><w:pPr>" + props + "</w:pPr>" + wordRuns(n) + "</w:p>")
			}
		}
	}
	parts := map[string]string{
		"[Content_Types].xml":          `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/></Types>`,
		"_rels/.rels":                  `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
		"word/styles.xml":              `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:eastAsia="宋体"/><w:sz w:val="24"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="480" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style><w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:rPr><w:b/><w:sz w:val="40"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:pPr><w:pageBreakBefore/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:sz w:val="32"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:pPr><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:b/><w:sz w:val="28"/></w:rPr></w:style></w:styles>`,
		"word/document.xml":            `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`,
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for name, part := range parts {
		if err := zipPart(z, name, part, false); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func xhtmlRuns(n richNode) string {
	if n.Type == "hardBreak" {
		return "<br/>"
	}
	if n.Type == "text" {
		s := xmlText(n.Text)
		for _, m := range n.Marks {
			switch m.Type {
			case "bold":
				s = "<strong>" + s + "</strong>"
			case "italic":
				s = "<em>" + s + "</em>"
			case "underline":
				s = `<span class="underline">` + s + "</span>"
			case "strike":
				s = "<s>" + s + "</s>"
			}
		}
		return s
	}
	var b strings.Builder
	for _, child := range n.Content {
		b.WriteString(xhtmlRuns(child))
	}
	return b.String()
}
func ExportEPUB(p Project) ([]byte, error) {
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	add := func(name, body string) error { return zipPart(z, name, body, false) }
	if err := zipPart(z, "mimetype", "application/epub+zip", true); err != nil {
		return nil, err
	}
	if err := add("META-INF/container.xml", `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`); err != nil {
		return nil, err
	}
	var manifest, spine, nav strings.Builder
	for i, c := range orderedChapters(p) {
		name := fmt.Sprintf("chapter-%d.xhtml", i+1)
		id := fmt.Sprintf("c%d", i+1)
		fmt.Fprintf(&manifest, `<item id="%s" href="%s" media-type="application/xhtml+xml"/>`, id, name)
		fmt.Fprintf(&spine, `<itemref idref="%s"/>`, id)
		fmt.Fprintf(&nav, `<li><a href="%s">%s</a></li>`, name, xmlText(c.Title))
		var body strings.Builder
		fmt.Fprintf(&body, "<h1>%s</h1>", xmlText(c.Title))
		for _, n := range richBlocks(c) {
			tag := "p"
			if n.Type == "heading" {
				tag = "h2"
			}
			fmt.Fprintf(&body, "<%s>%s</%s>", tag, xhtmlRuns(n), tag)
		}
		if err := add("EPUB/"+name, `<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xml:lang="zh-CN"><head><title>`+xmlText(c.Title)+`</title><link rel="stylesheet" type="text/css" href="style.css"/></head><body>`+body.String()+`</body></html>`); err != nil {
			return nil, err
		}
	}
	if err := add("EPUB/style.css", `body{font-family:serif;line-height:1.8}p{text-indent:2em}.underline{text-decoration:underline}`); err != nil {
		return nil, err
	}
	if err := add("EPUB/nav.xhtml", `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="zh-CN"><head><title>目录</title></head><body><nav epub:type="toc" id="toc"><h1>`+xmlText(p.Name)+`</h1><ol>`+nav.String()+`</ol></nav></body></html>`); err != nil {
		return nil, err
	}
	if err := add("EPUB/package.opf", `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="book-id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="book-id">urn:novel-edit:`+xmlText(p.ID)+`</dc:identifier><dc:title>`+xmlText(p.Name)+`</dc:title><dc:language>zh-CN</dc:language><meta property="dcterms:modified">`+Now()+`</meta></metadata><manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="css" href="style.css" media-type="text/css"/>`+manifest.String()+`</manifest><spine>`+spine.String()+`</spine></package>`); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
