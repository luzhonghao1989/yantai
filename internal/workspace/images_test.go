package workspace

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageFilesAndPortableBackup(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	p, _ := s.Create("图库", false)
	var raw bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.RGBA{G: 200, A: 255})
	png.Encode(&raw, im)
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><rect x="1" y="1" width="80" height="50" fill="#eee"/><text x="10" y="30">房间</text></svg>`
	p.Images = []ProjectImage{{ID: ID(), Name: "人物.png", Kind: "character", Data: "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw.Bytes())}, {ID: ID(), Name: "地图.svg", Kind: "map", SourceRunID: ID(), Data: "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))}}
	var err error
	p, err = s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range p.Images {
		if entry.Data != "" || entry.SHA == "" {
			t.Fatal("snapshot contains image bytes")
		}
		if _, err = os.Stat(filepath.Join(s.Root, "images", entry.SHA)); err != nil {
			t.Fatal(err)
		}
	}
	h := (&Server{Store: s, Token: "session"}).Handler()
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://localhost"+path, nil)
		req.Header.Set("X-Yantai-Token", "session")
		out := httptest.NewRecorder()
		h.ServeHTTP(out, req)
		return out
	}
	response := get("/api/projects/" + p.ID + "/images/" + p.Images[0].ID)
	if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), raw.Bytes()) {
		t.Fatal("PNG fetch failed")
	}
	response = get("/api/projects/" + p.ID + "/images/" + p.Images[1].ID)
	if response.Header().Get("Content-Type") != "image/svg+xml" || response.Body.String() != svg {
		t.Fatal("SVG fetch failed")
	}
	exported := get("/api/projects/" + p.ID + "/export?format=json")
	if exported.Code != 200 {
		t.Fatal(exported.Body.String())
	}
	var backup Project
	json.Unmarshal(exported.Body.Bytes(), &backup)
	if backup.Images[0].Data == "" || backup.Images[1].Data == "" {
		t.Fatal("backup omitted files")
	}
	target, _ := NewStore(t.TempDir())
	req := httptest.NewRequest("POST", "http://localhost/api/import", bytes.NewReader(exported.Body.Bytes()))
	req.Header.Set("Content-Type", "application/json")
	restored := httptest.NewRecorder()
	(&Server{Store: target}).Handler().ServeHTTP(restored, req)
	if restored.Code != 201 {
		t.Fatal(restored.Body.String())
	}
	var copied Project
	json.Unmarshal(restored.Body.Bytes(), &copied)
	if copied.Images[1].SourceRunID != p.Images[1].SourceRunID {
		t.Fatal("map origin lost")
	}
	data, _ := os.ReadFile(filepath.Join(target.Root, "images", copied.Images[0].SHA))
	if !bytes.Equal(data, raw.Bytes()) {
		t.Fatal("image did not travel to new store")
	}
	// Removing metadata does not delete assets needed by historical versions.
	old := p.Revision
	p.Images = nil
	p, err = s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.Restore(p.ID, old, p.Revision)
	if err != nil || len(p.Images) != 2 {
		t.Fatal("history restore lost images")
	}
	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest("GET", "http://localhost/api/projects/"+p.ID+"/images/"+p.Images[0].ID, nil))
	if bad.Code != 403 {
		t.Fatal("unauthenticated image accepted")
	}
}
func TestUnsafeImageRejection(t *testing.T) {
	for _, text := range []string{`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`, `<svg onload="alert(1)"/>`, `<svg><image href="https://evil.example/picture"/></svg>`, `<svg><rect fill="url(https://evil.example/a)"/></svg>`, `<!DOCTYPE svg><svg/>`, `<html><body>bad</body></html>`} {
		if _, _, err := decodeProjectImage("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(text))); err == nil {
			t.Fatal("unsafe SVG accepted", text)
		}
	}
	if _, _, err := decodeProjectImage("data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("fake png"))); err == nil {
		t.Fatal("fake PNG accepted")
	}
	if _, err := (&Store{Root: t.TempDir()}).imagePath("../../escape"); err == nil {
		t.Fatal("invalid image path accepted")
	}
	p := NewProject("", true)
	p.Images = []ProjectImage{{ID: ID(), Name: "bad", Kind: "map", SHA: strings.Repeat("a", 64), MIME: "text/html"}}
	if Validate(p) == nil {
		t.Fatal("HTML MIME accepted")
	}
}

func TestConflictDraftCopyPreservesLatestAndSharedImages(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("并发草稿", true)
	if err != nil {
		t.Fatal(err)
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>`
	p.Images = []ProjectImage{{ID: ID(), Name: "保留图.svg", Kind: "map", Data: "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))}}
	p, err = s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	draft := p
	draft.Chapters = append([]Chapter(nil), p.Chapters...)
	draft.Chapters[0].Text = "窗口独有正文"
	draft.Chapters[0].Content = EmptyDoc(draft.Chapters[0].Text)
	p.Chapters[0].Text = "API 最新正文"
	p.Chapters[0].Content = EmptyDoc(p.Chapters[0].Text)
	p, err = s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(draft); err != ErrConflict {
		t.Fatal("stale draft overwrote latest", err)
	}
	raw, _ := json.Marshal(draft)
	request := httptest.NewRequest("POST", "http://localhost/api/import", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	(&Server{Store: s}).Handler().ServeHTTP(response, request)
	if response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	var copy Project
	if err = json.Unmarshal(response.Body.Bytes(), &copy); err != nil {
		t.Fatal(err)
	}
	if copy.ID == p.ID || copy.Chapters[0].Text != "窗口独有正文" || len(copy.Images) != 1 || copy.Images[0].SHA != p.Images[0].SHA {
		t.Fatal("draft copy incomplete")
	}
	latest, err := s.Load(p.ID)
	if err != nil || latest.Revision != p.Revision || latest.Chapters[0].Text != "API 最新正文" {
		t.Fatal("original latest changed", err)
	}
}
