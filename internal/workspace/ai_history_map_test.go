package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReviewAndMapHistoryPersistence(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("报告保存", true)
	if err != nil {
		t.Fatal(err)
	}
	p.AIRuns = []AIRun{{ID: ID(), ChapterID: p.Chapters[0].ID, Mode: "review", Text: "保留此审稿意见", Model: "fixture", InputText: p.Chapters[0].Text, CreatedAt: Now()}, {ID: ID(), ChapterID: p.Chapters[0].ID, Mode: "map", Text: `{"title":"旧宅地图","nodes":[],"edges":[]}`, Model: "fixture", CreatedAt: Now()}}
	p, err = s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := s.Load(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.AIRuns) != 2 || reloaded.AIRuns[0].InputText != p.Chapters[0].Text || reloaded.AIRuns[0].Text != "保留此审稿意见" || reloaded.AIRuns[1].Mode != "map" {
		t.Fatal("saved history not restored")
	}
	handler := (&Server{Store: s}).Handler()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:48120/api/projects/"+p.ID+"/export?format=json", nil))
	req := httptest.NewRequest("POST", "http://127.0.0.1:48120/api/import", strings.NewReader(w.Body.String()))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var copied Project
	json.Unmarshal(w.Body.Bytes(), &copied)
	if len(copied.AIRuns) != 2 || copied.AIRuns[0].InputText != p.Chapters[0].Text {
		t.Fatal("backup import lost AI history")
	}
}
func TestSceneMapUsesCurrentContextAndMapPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Messages) != 2 || !strings.Contains(body.Messages[0].Content, "场景平面图规划助手") || !strings.Contains(body.Messages[0].Content, "uncertain") || !strings.Contains(body.Messages[1].Content, "青石镇旧宅") {
			t.Error("map prompt missing current scene or schema")
		}
		fmt.Fprintln(w, `{"message":{"content":"{\"title\":\"测试地图\",\"nodes\":[],\"edges\":[]}"},"done":true}`)
	}))
	defer server.Close()
	p := NewProject("", true)
	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.Model = "fixture"
	cp, err := BuildContext(p, AIRequest{ChapterID: p.Chapters[0].ID, Mode: "map", Instruction: "绘制当前旧宅"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Generate(context.Background(), cfg, cp, "map", func(string) error { return nil })
	if err != nil || !strings.Contains(out, "测试地图") {
		t.Fatalf("map generation: %v %s", err, out)
	}
}
