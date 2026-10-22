package workspace

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRevisionNoteBackupAndManuscriptExport(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("批注测试", false)
	if err != nil {
		t.Fatal(err)
	}
	p.Chapters[0].Text = "需要修改的对白。"
	p.Chapters[0].Content = json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"需要修改的对白。","marks":[{"type":"bold"},{"type":"revisionNote","attrs":{"id":"note-1","note":"内部修改说明不要混入正文","resolved":false}}]}]}]}`)
	p, err = s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Chapters[0].Content) != string(p.Chapters[0].Content) {
		t.Fatal("note lost on save/load")
	}
	handler := (&Server{Store: s}).Handler()
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, httptest.NewRequest("GET", "http://127.0.0.1:48120/api/projects/"+p.ID+"/export?format=json", nil))
	if out.Code != 200 {
		t.Fatal(out.Body.String())
	}
	req := httptest.NewRequest("POST", "http://127.0.0.1:48120/api/import", strings.NewReader(out.Body.String()))
	req.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, req)
	if result.Code != 201 {
		t.Fatalf("import: %d %s", result.Code, result.Body.String())
	}
	var imported Project
	if err = json.Unmarshal(result.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	var before, after any
	json.Unmarshal(p.Chapters[0].Content, &before)
	json.Unmarshal(imported.Chapters[0].Content, &after)
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatal("backup import lost note metadata")
	}
	for _, export := range []func(Project) ([]byte, error){ExportDOCX, ExportEPUB} {
		data, err := export(p)
		if err != nil {
			t.Fatal(err)
		}
		_, parts := zipContents(t, data)
		found := false
		for _, part := range parts {
			if strings.Contains(part, "内部修改说明") {
				t.Fatal("private note leaked into manuscript")
			}
			if strings.Contains(part, p.Chapters[0].Text) {
				found = true
			}
		}
		if !found {
			t.Fatal("marked manuscript text missing")
		}
	}
}
