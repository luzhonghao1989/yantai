package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func streamTestConfig(server *httptest.Server, provider string) AIConfig {
	cfg := DefaultConfig()
	cfg.Provider = provider
	cfg.BaseURL = server.URL
	cfg.Model = "fixture"
	cfg.ContextLimit = 262144
	cfg.OutputLimit = 32768
	return cfg
}
func TestActiveStreamOutlivesIdleTimeout(t *testing.T) {
	for _, provider := range []string{"compatible", "ollama"} {
		t.Run(provider, func(t *testing.T) {
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				json.NewDecoder(r.Body).Decode(&payload)
				if provider == "compatible" && payload["max_tokens"] != float64(32768) {
					t.Error("configured output limit lost")
				}
				for i := 0; i < 24; i++ {
					var value any = map[string]any{"message": map[string]string{"thinking": "思考"}}
					if provider == "compatible" {
						fmt.Fprint(w, "data: ")
						value = map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"reasoning_content": "思考"}}}}
					}
					json.NewEncoder(w).Encode(value)
					if provider == "compatible" {
						fmt.Fprint(w, "\n")
					}
					w.(http.Flusher).Flush()
					select {
					case <-time.After(50 * time.Millisecond):
					case <-r.Context().Done():
						return
					}
				}
				if provider == "compatible" {
					fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"审稿完成"},"finish_reason":"stop"}]}

`)
				} else {
					fmt.Fprintln(w, `{"message":{"content":"审稿完成"},"done":true}`)
				}
			}))
			defer model.Close()
			stats := modelStreamStats{}
			start := time.Now()
			out, err := generateWithTimeouts(context.Background(), streamTestConfig(model, provider), ContextPackage{Fits: true}, "review", func(string) error { return nil }, nil, time.Second, 0, &stats)
			if err != nil || out != "审稿完成" || time.Since(start) <= time.Second || stats.ThinkingCharacters != 48 {
				t.Fatalf("active stream cut off: %q %v %#v", out, err, stats)
			}
		})
	}
}
func TestIdleTotalCancellationAndTruncation(t *testing.T) {
	for _, scenario := range []string{"idle", "total", "cancel", "eof", "length", "http-context", "headers"} {
		t.Run(scenario, func(t *testing.T) {
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				if scenario == "headers" {
					select {
					case <-r.Context().Done():
					case <-time.After(2 * time.Second):
					}
					return
				}
				if scenario == "http-context" {
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":{"code":"context_length_exceeded","message":"maximum context length rejected"}}`)
					return
				}
				fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"部分结果"}}]}

`)
				w.(http.Flusher).Flush()
				if scenario == "eof" {
					return
				}
				if scenario == "length" {
					fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":28746,"completion_tokens":32768}}

`)
					return
				}
				if scenario == "total" {
					for {
						select {
						case <-r.Context().Done():
							return
						case <-time.After(15 * time.Millisecond):
							fmt.Fprint(w, ": heartbeat\n\n")
							w.(http.Flusher).Flush()
						}
					}
				}
				<-r.Context().Done()
			}))
			defer model.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			idle, total := 100*time.Millisecond, time.Duration(0)
			if scenario == "total" {
				idle = time.Second
				total = 120 * time.Millisecond
			}
			if scenario == "cancel" {
				idle = 0
				time.AfterFunc(50*time.Millisecond, cancel)
			}
			stats := modelStreamStats{}
			out, err := generateWithTimeouts(ctx, streamTestConfig(model, "compatible"), ContextPackage{Fits: true}, "review", func(string) error { return nil }, nil, idle, total, &stats)
			if err == nil {
				t.Fatal("incomplete response accepted")
			}
			if scenario != "http-context" && scenario != "headers" && out != "部分结果" {
				t.Fatal("partial answer lost", out, err)
			}
			expected := map[string]string{"idle": "idle_timeout", "total": "total_timeout", "eof": "connection_closed", "length": "output_limit", "http-context": "service_context_limit", "headers": "idle_timeout"}
			if scenario == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else {
				var failure *modelFailureError
				if !errors.As(err, &failure) || failure.Kind != expected[scenario] {
					t.Fatalf("wrong error %v", err)
				}
			}
			if scenario == "length" && (stats.InputTokens != 28746 || stats.OutputTokens != 32768) {
				t.Fatal("token usage lost", stats)
			}
		})
	}
}
func TestAIErrorLogPrivacyPersistenceAndAuthorization(t *testing.T) {
	secret, prose, thinking := "secret-fixture-key", "正文不要记录", "思考不要记录"
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":%q,\"content\":%q}}]}\n\n", thinking, prose)
	}))
	defer model.Close()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := streamTestConfig(model, "compatible")
	cfg.APIKey = secret
	cfg.RequestTimeoutMinutes = 0
	cfg.IdleTimeoutMinutes = 7
	if _, err = store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Config()
	if err != nil || loaded.IdleTimeoutMinutes != 7 || loaded.RequestTimeoutMinutes != 0 {
		t.Fatal("timeouts not persisted", err)
	}
	cp := ContextPackage{Fits: true, ChapterID: ID(), EstimatedTokens: 28746, Sections: []ContextSection{{Text: prose}}}
	_, err = Generate(context.Background(), loaded, cp, "review", func(string) error { return nil })
	if err == nil {
		t.Fatal("missing completion accepted")
	}
	records, err := store.aiErrors()
	if err != nil || len(records) != 1 || records[0].Kind != "connection_closed" || records[0].EstimatedInputTokens != 28746 || records[0].ThinkingCharacters != len([]rune(thinking)) {
		t.Fatalf("wrong record %#v %v", records, err)
	}
	raw, err := os.ReadFile(filepath.Join(store.Root, "logs", "ai-errors.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{secret, prose, thinking} {
		if strings.Contains(string(raw), value) {
			t.Fatal("private content leaked", value)
		}
	}
	again, _ := NewStore(store.Root)
	persisted, _ := again.aiErrors()
	if len(persisted) != 1 {
		t.Fatal("error log not durable")
	}
	handler := (&Server{Store: store, Token: "fixture-session"}).Handler()
	for _, authorized := range []bool{false, true} {
		r := httptest.NewRequest("GET", "http://localhost/api/ai/errors", nil)
		if authorized {
			r.Header.Set("X-Yantai-Token", "fixture-session")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if authorized && w.Code != 200 || !authorized && w.Code != 403 {
			t.Fatalf("log auth %t: %d", authorized, w.Code)
		}
	}
	// Provider errors can echo prompts: persist only a safe code and fixed summary.
	providerError := modelServiceFailure([]byte(`{"code":"context_length_exceeded","message":"`+secret+` `+prose+`"}`), secret, "error: ")
	record := newAIErrorRecord(cfg, cp, "review", time.Now(), modelStreamStats{HTTPStatus: 400}, providerError)
	raw, _ = json.Marshal(record)
	if record.ServiceCode != "context_length_exceeded" || strings.Contains(string(raw), secret) || strings.Contains(string(raw), prose) {
		t.Fatal("unsafe provider error log", string(raw))
	}
}
func TestAIErrorLogRotationAndTimeoutValidation(t *testing.T) {
	store, _ := NewStore(t.TempDir())
	for i := 0; i < 4500; i++ {
		if err := store.recordAIError(AIErrorRecord{ID: fmt.Sprint(i), Message: strings.Repeat("x", 900)}); err != nil {
			t.Fatal(err)
		}
	}
	records, err := store.aiErrors()
	if err != nil || len(records) != 100 || records[0].ID != "4499" || records[99].ID != "4400" {
		t.Fatal("retention failed", len(records), err)
	}
	files, err := os.ReadDir(filepath.Join(store.Root, "logs"))
	if err != nil || len(files) > 4 {
		t.Fatal("unbounded log files", err)
	}
	for _, minutes := range []int{-1, 0, 1, 60} {
		cfg := DefaultConfig()
		cfg.RequestTimeoutMinutes = minutes
		cfg.IdleTimeoutMinutes = minutes
		err := ValidateConfig(cfg)
		if (minutes < 0) != (err != nil) {
			t.Fatal("wrong timeout validation", minutes, err)
		}
	}
}

func TestBudgetErrorsAreLoggedAndPartialSSEIsNotComplete(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"未完成的候选"}}]}

`)
	}))
	defer model.Close()
	store, _ := NewStore(t.TempDir())
	p, _ := store.Create("审稿测试", true)
	cfg := streamTestConfig(model, "compatible")
	store.SaveConfig(cfg)
	request := func() *httptest.ResponseRecorder {
		raw, _ := json.Marshal(AIRequest{ChapterID: p.Chapters[0].ID, Revision: p.Revision, Mode: "review"})
		r := httptest.NewRequest("POST", "http://localhost/api/projects/"+p.ID+"/generate", strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		(&Server{Store: store}).Handler().ServeHTTP(w, r)
		return w
	}
	w := request()
	if !strings.Contains(w.Body.String(), `"type":"delta"`) || !strings.Contains(w.Body.String(), `"type":"error"`) || strings.Contains(w.Body.String(), `"type":"done"`) {
		t.Fatal("partial stream marked complete", w.Body.String())
	}
	cfg.ContextLimit = 1024
	cfg.OutputLimit = 128
	store.SaveConfig(cfg)
	w = request()
	if w.Code != 400 || !strings.Contains(w.Body.String(), "输出预留") {
		t.Fatal("budget diagnostics missing", w.Code, w.Body.String())
	}
	records, err := store.aiErrors()
	if err != nil || len(records) != 2 || records[0].Kind != "context_limit" || records[1].Kind != "connection_closed" {
		t.Fatal("budget/stream log missing", records, err)
	}
	legacy, _ := NewStore(t.TempDir())
	if err = os.WriteFile(filepath.Join(legacy.Root, "settings.json"), []byte(`{"provider":"ollama","baseUrl":"http://localhost:11434","contextLimit":262144,"outputLimit":32768}`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := legacy.Config()
	if err != nil || loaded.RequestTimeoutMinutes != 0 || loaded.IdleTimeoutMinutes != 10 {
		t.Fatal("legacy defaults lost", loaded.RequestTimeoutMinutes, loaded.IdleTimeoutMinutes, err)
	}
}
