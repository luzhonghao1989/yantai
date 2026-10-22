package workspace

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestPortableProjectBackup(t *testing.T) {
	source, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := source.Create("跨电脑备份", true)
	if err != nil {
		t.Fatal(err)
	}
	p.Entities[0].Profile = map[string]string{"gender": "女", "birthplace": "青石镇"}
	p.Sources = append(p.Sources, Source{ID: ID(), Title: "资料", Text: "参考资料", URL: "https://example.com"})
	p.AIRuns = append(p.AIRuns, AIRun{ID: ID(), ChapterID: p.Chapters[0].ID, Mode: "continue", Model: "test", Text: "候选", Accepted: true, CreatedAt: Now()})
	p, err = source.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.APIKey = "secret-not-in-backup"
	source.SaveConfig(cfg)
	exported := httptest.NewRecorder()
	(&Server{Store: source}).Handler().ServeHTTP(exported, httptest.NewRequest("GET", "http://localhost/api/projects/"+p.ID+"/export?format=json", nil))
	if exported.Code != 200 || bytes.Contains(exported.Body.Bytes(), []byte(cfg.APIKey)) {
		t.Fatal("backup export failed or contains credentials")
	}
	target, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "http://localhost/api/import", bytes.NewReader(exported.Body.Bytes()))
	req.Header.Set("Content-Type", "application/json")
	imported := httptest.NewRecorder()
	(&Server{Store: target}).Handler().ServeHTTP(imported, req)
	if imported.Code != 201 {
		t.Fatal(imported.Body.String())
	}
	var restored Project
	json.Unmarshal(imported.Body.Bytes(), &restored)
	if restored.ID == p.ID {
		t.Fatal("import reused original project identity")
	}
	// Storage metadata and the import name change; every project record remains portable.
	restored.ID = p.ID
	restored.Name = p.Name
	restored.Revision = p.Revision
	restored.UpdatedAt = p.UpdatedAt
	if !reflect.DeepEqual(restored, p) {
		t.Fatal("portable backup lost project content")
	}
	if _, err := target.Load(restored.ID); err == nil {
		t.Fatal("original project unexpectedly exists on target")
	}
}
