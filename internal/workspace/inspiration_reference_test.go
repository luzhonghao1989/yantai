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

func TestRSSSearchSourcesAndInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/invalid" {
			fmt.Fprint(w, `<html>challenge</html>`)
			return
		}
		fmt.Fprint(w, `<rss><channel><item><title>雨夜 &amp; 来信</title><link>https://example.org/story</link><description>门外的雨声。</description></item><item><title>invalid</title><link>javascript:alert(1)</link></item></channel></rss>`)
	}))
	defer server.Close()
	out, err := searchRSS(context.Background(), server.URL)
	if err != nil || len(out) != 1 || out[0].Title != "雨夜 & 来信" || out[0].URL != "https://example.org/story" {
		t.Fatalf("sources: %+v, %v", out, err)
	}
	if _, err := searchRSS(context.Background(), server.URL+"/invalid"); err == nil {
		t.Fatal("accepted challenge as search results")
	}
}

func TestStyleReferenceIncludesSourcesAndCurrentPlanning(t *testing.T) {
	p := NewProject("", true)
	p.Chapters[0].Summary = "前情测试：主人公收到来信"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		prompt := payload.Messages[1].Content
		for _, expected := range []string{"前情测试", p.Chapters[0].Outline, "[1]", "https://example.org/reference", "实际参考摘要"} {
			if !strings.Contains(prompt, expected) {
				t.Errorf("missing context: %s", expected)
			}
		}
		fmt.Fprintln(w, `{"message":{"content":"参考写法 [1]；原创示例：她合上信。"},"done":true}`)
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.Model = "test"
	cfg.BaseURL = server.URL
	found := InspirationResult{Queries: []string{"来信"}, Results: []SearchResult{{Title: "参考标题", URL: "https://example.org/reference", Text: "实际参考摘要"}}}
	out, err := synthesizeStyle(context.Background(), cfg, p, InspirationRequest{AIRequest: AIRequest{ChapterID: p.Chapters[0].ID}, Query: "来信桥段"}, found)
	if err != nil || !strings.Contains(out.Analysis, "原创示例") || len(out.Results) != 1 {
		t.Fatalf("reference failed: %+v, %v", out, err)
	}
	cfg.ContextLimit = cfg.OutputLimit + 10
	if _, err := synthesizeStyle(context.Background(), cfg, p, InspirationRequest{AIRequest: AIRequest{ChapterID: p.Chapters[0].ID}}, found); err == nil {
		t.Fatal("source context bypassed token budget")
	}
	if _, err := synthesizeStyle(context.Background(), cfg, p, InspirationRequest{}, InspirationResult{}); err == nil {
		t.Fatal("allowed invented references without sources")
	}
}
