package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestThinkingTagsAcrossChunks(t *testing.T) {
	for _, input := range []string{"<think>核对人物和阶段。</think>正式改文。", "<thinking>核对设定</thinking>正式改文。", "<THINK>核对设定</THINK>正式改文。", "İ<think>核对</think>正式改文。"} {
		for split := 0; split <= len(input); split++ {
			var answer, thought strings.Builder
			stream := thinkingStream{answer: func(s string) error { answer.WriteString(s); return nil }, thought: func(s string) error { thought.WriteString(s); return nil }}
			for _, part := range []string{input[:split], input[split:]} {
				if err := stream.write(part, false); err != nil {
					t.Fatal(err)
				}
			}
			if err := stream.write("", true); err != nil {
				t.Fatal(err)
			}
			expected := "正式改文。"
			if strings.HasPrefix(input, "İ") {
				expected = "İ" + expected
			}
			if answer.String() != expected || thought.Len() == 0 {
				t.Fatalf("split %d: %q / %q", split, answer.String(), thought.String())
			}
		}
	}
	var answer, thought strings.Builder
	stream := thinkingStream{answer: func(s string) error { answer.WriteString(s); return nil }, thought: func(s string) error { thought.WriteString(s); return nil }}
	for _, char := range "<think>尚未结束的思考" {
		if err := stream.write(string(char), false); err != nil {
			t.Fatal(err)
		}
	}
	_ = stream.write("", true)
	if answer.Len() != 0 || thought.String() != "尚未结束的思考" {
		t.Fatal("unclosed reasoning leaked")
	}
}

func TestThinkingProviderStreams(t *testing.T) {
	for _, provider := range []string{"ollama", "compatible"} {
		for _, field := range []string{"thinking", "reasoning_content", "reasoning", "tags", "none", "thought-only"} {
			if provider == "ollama" && field != "thinking" && field != "tags" && field != "none" && field != "thought-only" {
				continue
			}
			t.Run(provider+"/"+field, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					send := func(delta map[string]string) {
						var chunk any = map[string]any{"message": delta}
						if provider == "compatible" {
							chunk = map[string]any{"choices": []any{map[string]any{"delta": delta}}}
							fmt.Fprint(w, "data: ")
						}
						_ = json.NewEncoder(w).Encode(chunk)
						if provider == "compatible" {
							fmt.Fprint(w, "\n")
						}
					}
					switch field {
					case "tags":
						for _, part := range []string{"<thi", "nk>核对设定。", "</th", "ink>", "正式改文。"} {
							send(map[string]string{"content": part})
						}
					case "none":
						send(map[string]string{"content": "正式改文。"})
					default:
						f := field
						if f == "thought-only" {
							f = "thinking"
						}
						send(map[string]string{f: "核对设定。"})
						if field != "thought-only" {
							send(map[string]string{"content": "正式改文。"})
						}
					}
					if provider == "compatible" {
						fmt.Fprint(w, "data: [DONE]\n\n")
					}
				}))
				defer server.Close()
				cfg := DefaultConfig()
				cfg.Provider = provider
				cfg.BaseURL = server.URL
				cfg.Model = "fixture"
				cp := ContextPackage{Fits: true}
				var answer, thought strings.Builder
				out, err := GenerateWithThinking(context.Background(), cfg, cp, "continue", func(s string) error { answer.WriteString(s); return nil }, func(s string) error { thought.WriteString(s); return nil })
				if field == "thought-only" {
					if err == nil || out != "" {
						t.Fatalf("thought-only accepted: %q %v", out, err)
					}
					return
				}
				if err != nil || out != "正式改文。" || answer.String() != out {
					t.Fatalf("wrong formal output %q %q %v", out, answer.String(), err)
				}
				if field != "none" && thought.String() != "核对设定。" {
					t.Fatalf("missing thinking %q", thought.String())
				}
				out, err = Generate(context.Background(), cfg, cp, "continue", func(string) error { return nil })
				if err != nil || out != "正式改文。" {
					t.Fatalf("non-progress caller leaked thought: %q %v", out, err)
				}
				if field != "none" {
					stop := errors.New("consumer cancelled")
					_, err = GenerateWithThinking(context.Background(), cfg, cp, "continue", func(string) error { return nil }, func(string) error { return stop })
					if !errors.Is(err, stop) {
						t.Fatalf("thinking cancellation lost: %v", err)
					}
				}
			})
		}
	}
}

func TestThinkingSSESeparatesFinalOutput(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"message":{"thinking":"仅用于等待时查看"}}`)
		fmt.Fprintln(w, `{"message":{"content":"正式结果"},"done":true}`)
	}))
	defer model.Close()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.Create("thinking fixture", true)
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.BaseURL = model.URL
	cfg.Model = "thinking-fixture"
	if _, err = store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(AIRequest{ChapterID: p.Chapters[0].ID, Revision: p.Revision, Mode: "continue"})
	r := httptest.NewRequest("POST", "http://localhost/api/projects/"+p.ID+"/generate", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	(&Server{Store: store}).Handler().ServeHTTP(w, r)
	var formal string
	seenThinking, seenDone := false, false
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		switch event.Type {
		case "thinking":
			seenThinking = true
		case "delta":
			var part string
			_ = json.Unmarshal(event.Data, &part)
			formal += part
		case "done":
			seenDone = true
			if strings.Contains(string(event.Data), "仅用于等待时查看") {
				t.Fatal("thought persisted in final event")
			}
		case "error":
			t.Fatalf("stream failed: %s", event.Data)
		}
	}
	if !seenThinking || !seenDone || formal != "正式结果" {
		t.Fatalf("missing separated stream: %s", w.Body.String())
	}
	restored, err := store.Load(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.AIRuns) != len(p.AIRuns) || restored.Chapters[0].Text != p.Chapters[0].Text {
		t.Fatal("progress changed stored project")
	}
	for _, raw := range []string{`null`, `false`, `{"encrypted_content":"hidden"}`, `[{"type":"metadata"}]`} {
		if modelThinkingText(json.RawMessage(raw)) != "" {
			t.Fatal("metadata exposed")
		}
	}
}
