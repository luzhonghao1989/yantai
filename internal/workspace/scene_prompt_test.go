package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChapterScenePromptReadsProseAndCurrentContext(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
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
			t.Error("missing scene messages")
			return
		}
		if !strings.Contains(body.Messages[0].Content, "小说场景美术指导") || !strings.Contains(body.Messages[0].Content, "negativePrompt") {
			t.Error("missing visual prompt contract")
		}
		prompt := body.Messages[1].Content
		for _, want := range []string{"石缸旁的一封旧信", "青石镇旧宅", "章纲", "当前阶段细纲", "俯视，不画人物"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("missing chapter context: %s", want)
			}
		}
		if strings.LastIndex(prompt, "## 任务") < strings.LastIndex(prompt, "## 当前正文") {
			t.Error("visual task buried before prose")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": "```json\n{\"title\":\"雨后旧宅\",\"description\":\"依据本章旧宅、石缸与窗外窄巷\",\"prompt\":\"雨后的青石镇旧宅，石缸旁的旧信，俯视构图\",\"negativePrompt\":\"水印，文字\"}\n```"}, "done": true})
	}))
	defer server.Close()
	p := NewProject("test", true)
	p.Chapters[0].Text = "雨停后，林晚看见石缸旁的一封旧信。"
	cfg := DefaultConfig()
	cfg.BaseURL, cfg.Model = server.URL, "fixture"
	req := scenePromptRequest{ChapterID: p.Chapters[0].ID, Revision: p.Revision, Instruction: "俯视，不画人物"}
	for _, entity := range p.Entities {
		if entity.Kind == "location" {
			req.EntityID = entity.ID
			break
		}
	}
	out, err := generateChapterScenePrompt(context.Background(), cfg, p, req)
	if err != nil || out.Prompt != "雨后的青石镇旧宅，石缸旁的旧信，俯视构图" || out.NegativePrompt != "水印，文字" {
		t.Fatalf("scene prompt: %v %#v", err, out)
	}
	req.Revision--
	if _, err = generateChapterScenePrompt(context.Background(), cfg, p, req); err != ErrConflict {
		t.Fatal("stale chapter accepted")
	}
	req.Revision = p.Revision
	req.EntityID = "not-bound"
	if _, err = generateChapterScenePrompt(context.Background(), cfg, p, req); err == nil {
		t.Fatal("unbound scene accepted")
	}
	req.EntityID = ""
	p.Chapters[0].Text = ""
	if _, err = generateChapterScenePrompt(context.Background(), cfg, p, req); err == nil {
		t.Fatal("empty chapter accepted")
	}
	if calls != 1 {
		t.Fatal("invalid inputs requested a model")
	}
}
func TestScenePromptValidation(t *testing.T) {
	for _, text := range []string{"not JSON", `{"prompt":""}`, `{"prompt":42}`, `{"prompt":"x","negativePrompt":[]}`} {
		if _, err := parseScenePrompt(text); err == nil {
			t.Fatalf("invalid scene accepted: %s", text)
		}
	}
	out, err := parseScenePrompt(`{"prompt":"旧宅，雨后"}`)
	if err != nil || out.Title != "本章场景图" {
		t.Fatal("optional title fallback failed")
	}
}
