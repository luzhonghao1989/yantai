package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAIReviewRequestsReplaceableParagraphs(t *testing.T) {
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
			t.Error("missing review messages")
			return
		}
		for _, word := range []string{"中文小说审稿编辑", "完整原段落", "original", "replacement", "reason", "不能原样返回"} {
			if !strings.Contains(body.Messages[0].Content, word) {
				t.Errorf("missing replaceable review contract: %s", word)
			}
		}
		if !strings.Contains(body.Messages[1].Content, "对白更口语") {
			t.Error("missing author review advice")
		}
		answer := `{"summary":"对白建议","suggestions":[{"paragraph":1,"original":"她说你好。","replacement":"她抬眼问：你来了？","reason":"对白更自然"}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": answer}, "done": true})
	}))
	defer server.Close()
	p := NewProject("test", true)
	cfg := DefaultConfig()
	cfg.BaseURL, cfg.Model = server.URL, "fixture"
	cp, err := BuildContext(p, AIRequest{ChapterID: p.Chapters[0].ID, Mode: "review", Instruction: "根据已有审稿意见让对白更口语"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Generate(context.Background(), cfg, cp, "review", func(string) error { return nil })
	if err != nil || !strings.Contains(out, "replacement") {
		t.Fatalf("structured review: %v %s", err, out)
	}
}
