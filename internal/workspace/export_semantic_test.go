package workspace

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func zipContents(t *testing.T, data []byte) (*zip.Reader, map[string]string) {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name] = string(b)
		if strings.HasSuffix(f.Name, "xml") || strings.HasSuffix(f.Name, "rels") || strings.HasSuffix(f.Name, "opf") || strings.HasSuffix(f.Name, "xhtml") {
			d := xml.NewDecoder(bytes.NewReader(b))
			for {
				_, err := d.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("invalid XML %s: %v", f.Name, err)
				}
			}
		}
	}
	return r, files
}
func TestRichDocumentExports(t *testing.T) {
	p := NewProject("测试<&>小说", false)
	p.Chapters[0].Title = "第一章 <雨>"
	p.Chapters[0].Content = json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"你好<&>世界","marks":[{"type":"bold"},{"type":"italic"},{"type":"textStyle","attrs":{"fontFamily":"楷体"}}]},{"type":"hardBreak"},{"type":"text","text":"第二行"}]}]}`)
	data, err := ExportDOCX(p)
	if err != nil {
		t.Fatal(err)
	}
	_, files := zipContents(t, data)
	for _, want := range []string{"你好&lt;&amp;&gt;世界", "<w:b/>", "<w:i/>", "楷体", "<w:br/>", "第二行"} {
		if !strings.Contains(files["word/document.xml"], want) {
			t.Errorf("DOCX missing %s", want)
		}
	}
	data, err = ExportEPUB(p)
	if err != nil {
		t.Fatal(err)
	}
	r, files := zipContents(t, data)
	if r.File[0].Name != "mimetype" || r.File[0].Method != zip.Store || files["mimetype"] != "application/epub+zip" {
		t.Fatal("invalid EPUB container")
	}
	for _, want := range []string{"<strong>", "<em>", "你好&lt;&amp;&gt;世界", "<br/>", "第二行"} {
		if !strings.Contains(files["EPUB/chapter-1.xhtml"], want) {
			t.Errorf("EPUB missing %s", want)
		}
	}
	if !strings.Contains(files["EPUB/nav.xhtml"], "chapter-1.xhtml") || !strings.Contains(files["EPUB/package.opf"], `properties="nav"`) {
		t.Fatal("missing EPUB navigation")
	}
}
func TestSemanticSearchActualSourcesCacheAndStages(t *testing.T) {
	for _, provider := range []string{"ollama", "compatible"} {
		t.Run(provider, func(t *testing.T) {
			batches := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var in struct {
					Input []string `json:"input"`
				}
				json.NewDecoder(r.Body).Decode(&in)
				if len(in.Input) > 1 {
					batches++
				}
				vectors := [][]float64{}
				for _, s := range in.Input {
					v := []float64{0, 1}
					if strings.Contains(s, "眼泪") || s == "悲伤" {
						v = []float64{1, 0}
					}
					vectors = append(vectors, v)
				}
				if provider == "ollama" {
					json.NewEncoder(w).Encode(map[string]any{"embeddings": vectors})
				} else {
					rows := []map[string]any{}
					for i := len(vectors) - 1; i >= 0; i-- {
						rows = append(rows, map[string]any{"index": i, "embedding": vectors[i]})
					}
					json.NewEncoder(w).Encode(map[string]any{"data": rows})
				}
			}))
			defer srv.Close()
			cfg := DefaultConfig()
			cfg.Provider = provider
			cfg.BaseURL = srv.URL
			cfg.EmbeddingModel = "fixture"
			p := NewProject("", true)
			p.Chapters[0].Text = "她擦去眼泪。\n风吹过窗户。"
			future := p.Chapters[0]
			future.ID = ID()
			future.StageID = p.Stages[1].ID
			future.Text = "未来章：她擦去眼泪。"
			p.Chapters = append(p.Chapters, future)
			store, _ := NewStore(t.TempDir())
			for n := 0; n < 2; n++ {
				out, err := store.SemanticSearch(context.Background(), cfg, p, "悲伤", p.Chapters[0].ID)
				if err != nil {
					t.Fatal(err)
				}
				if len(out) != 2 || out[0].Text != "她擦去眼泪。" || out[0].Score != 1000 {
					t.Fatalf("wrong semantic ranking: %+v", out)
				}
			}
			if batches != 1 {
				t.Fatalf("source embeddings not cached: %d", batches)
			}
			p.Chapters[0].Text = "风吹过窗户。"
			out, err := store.SemanticSearch(context.Background(), cfg, p, "悲伤", p.Chapters[0].ID)
			if err != nil || len(out) != 1 || out[0].Text != "风吹过窗户。" {
				t.Fatal("stale source retained", err)
			}
		})
	}
}
func TestEmbeddingRejectsInvalidData(t *testing.T) {
	for _, body := range []string{`{"embeddings":[]}`, `{"embeddings":[[]]}`, `{"embeddings":[[0,0]]}`, `{"embeddings":[[1],[1,2]]}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
		cfg := DefaultConfig()
		cfg.BaseURL = srv.URL
		cfg.EmbeddingModel = "fixture"
		if _, err := embedTexts(context.Background(), cfg, []string{"one"}); err == nil {
			t.Fatal("accepted invalid embedding", body)
		}
		srv.Close()
	}
}
