package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTitleGenerationUsesChapterAndDoesNotEditIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if len(body.Messages) != 2 {
			t.Error("missing messages")
			return
		}
		if !strings.Contains(body.Messages[0].Content, `"titles"`) || !strings.Contains(body.Messages[1].Content, "城门已经打开") {
			t.Error("missing title contract or chapter prose")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": `{"titles":["第5章 城门开了","第5章 门后的脚步"]}`}, "done": true})
	}))
	defer server.Close()
	p := NewProject("title-test", true)
	p.Chapters[0].Title, p.Chapters[0].Text = "第5章 旧标题", "城门已经打开"
	before, _ := json.Marshal(p)
	cfg := DefaultConfig()
	cfg.BaseURL, cfg.Model = server.URL, "fixture"
	cp, err := BuildContext(p, AIRequest{ChapterID: p.Chapters[0].ID, Mode: "title"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Generate(context.Background(), cfg, cp, "title", func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Titles []string `json:"titles"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Titles) != 2 {
		t.Fatalf("invalid candidates: %s (%v)", out, err)
	}
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("title generation modified project")
	}
}
