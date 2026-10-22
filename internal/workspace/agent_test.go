package workspace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentAtomicUpsert(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.Create("接口测试", true)
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store, Token: "test-token"}).Handler()
	send := func(payload any, token string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(payload)
		r := httptest.NewRequest(http.MethodPost, "http://localhost/api/projects/"+p.ID+"/agent/apply", bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Yantai-Token", token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	payload := map[string]any{"revision": p.Revision, "outline": "新的总纲", "chapters": []any{map[string]any{"id": p.Chapters[0].ID, "title": "第一章 · 新标题", "text": "第一段。\n第二段。"}}, "entities": []any{map[string]any{"id": p.Entities[0].ID, "profile": map[string]string{"gender": "女", "birthplace": "青石镇"}}}}
	if w := send(payload, ""); w.Code != 403 {
		t.Fatalf("token guard: %d", w.Code)
	}
	w := send(payload, "test-token")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var updated Project
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Revision != p.Revision+1 || updated.Chapters[0].Outline != p.Chapters[0].Outline || len(updated.Entities) != len(p.Entities) || len(updated.Entities[0].Versions) != 2 {
		t.Fatal("upsert erased omitted data")
	}
	if !strings.Contains(string(updated.Chapters[0].Content), "第二段") {
		t.Fatal("plain text was not converted")
	}
	cp, err := BuildContext(updated, AIRequest{ChapterID: updated.Chapters[0].ID}, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(cp)
	if !strings.Contains(string(b), "出生地：青石镇") {
		t.Fatal("profile absent in AI context")
	}
	if w := send(payload, "test-token"); w.Code != 409 {
		t.Fatalf("stale revision: %d", w.Code)
	}
	invalid := map[string]any{"revision": updated.Revision, "outline": "不应保存", "chapters": []any{map[string]any{"id": ID(), "title": "非法引用", "stageId": "missing", "volumeId": p.Volumes[0].ID, "text": "正文"}}}
	if w := send(invalid, "test-token"); w.Code != 400 {
		t.Fatalf("bad reference: %d", w.Code)
	}
	unchanged, _ := store.Load(p.ID)
	if unchanged.Outline != "新的总纲" || unchanged.Revision != updated.Revision {
		t.Fatal("invalid batch partially saved")
	}
	added := map[string]any{"revision": updated.Revision, "chapters": []any{map[string]any{"title": "第二章", "volumeId": p.Volumes[0].ID, "stageId": p.Stages[0].ID, "text": "新章正文"}}}
	w = send(added, "test-token")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &updated)
	if len(updated.Chapters) != 2 || updated.Chapters[1].ID == "" {
		t.Fatal("new record ID missing")
	}
}
