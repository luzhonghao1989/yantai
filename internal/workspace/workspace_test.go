package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestStageResolutionAndContext(t *testing.T) {
	p := NewProject("", true)
	c := p.Chapters[0]
	early := Resolve(p, c)
	if len(early) != 3 {
		t.Fatalf("want 3 cards, got %d", len(early))
	}
	if strings.Contains(early[0].Version.State, "学会") {
		t.Fatal("future skill leaked")
	}
	c.StageID = p.Stages[1].ID
	late := Resolve(p, c)
	if !strings.Contains(late[0].Version.State, "学会") {
		t.Fatal("later version absent")
	}
	if !late[1].Inherited {
		t.Fatal("past object state not inherited")
	}
	req := AIRequest{ChapterID: p.Chapters[0].ID, Mode: "continue", Instruction: "继续"}
	cp, err := BuildContext(p, req, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cp.Prompt(), "学会铜铃辨雾") {
		t.Fatal("future entity state entered early context")
	}
	if !cp.Fits {
		t.Fatal("small demo should fit")
	}
	cfg := DefaultConfig()
	cfg.ContextLimit = 2200
	cp, _ = BuildContext(p, req, cfg)
	if cp.Fits {
		t.Fatal("oversized required context must block")
	}
	p.Chapters[0].ExcludedIDs = []string{p.Entities[0].ID}
	if len(Resolve(p, p.Chapters[0])) != 2 {
		t.Fatal("explicit chapter exclusion ignored")
	}
}
func TestSaveConflictIsolationAndRecovery(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	p, err := s.Create("甲", false)
	if err != nil {
		t.Fatal(err)
	}
	q, err := s.Create("乙", false)
	if err != nil {
		t.Fatal(err)
	}
	original := JSONCopy(p)
	p.Chapters[0].Text = "不会丢失的中文"
	p.Chapters[0].Content = EmptyDoc(p.Chapters[0].Text)
	updated, err := s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(original); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write accepted: %v", err)
	}
	other, _ := s.Load(q.ID)
	if other.Chapters[0].Text != "" {
		t.Fatal("projects contaminated")
	}
	loaded, _ := s.Load(p.ID)
	if loaded.Chapters[0].Text != p.Chapters[0].Text {
		t.Fatal("saved text lost")
	}
	hist, _ := s.History(p.ID)
	if len(hist) != 1 {
		t.Fatal("history absent")
	}
	restored, err := s.Restore(p.ID, hist[0].Revision, updated.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Chapters[0].Text != "" {
		t.Fatal("restore didn't recover old text")
	}
	if restored.Revision <= updated.Revision {
		t.Fatal("restoration reused stale revision")
	}
	if _, err = s.Load("../../x"); err == nil {
		t.Fatal("path traversal accepted")
	}
}
func TestConcurrentSaveOnlyOneSucceeds(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	p, _ := s.Create("并发", false)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Save(p); errs <- err }()
	}
	wg.Wait()
	close(errs)
	ok, conflict := 0, 0
	for err := range errs {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("ok=%d conflict=%d", ok, conflict)
	}
}
func TestProviderStreaming(t *testing.T) {
	for _, provider := range []string{"ollama", "compatible"} {
		t.Run(provider, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "test-model" {
					t.Error("wrong model")
				}
				if provider == "ollama" {
					fmt.Fprintln(w, `{"message":{"content":"你好"},"done":false}`)
					fmt.Fprintln(w, `{"message":{"content":"世界"},"done":true}`)
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"世界\"}}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer srv.Close()
			cfg := DefaultConfig()
			cfg.Provider = provider
			cfg.BaseURL = srv.URL
			cfg.Model = "test-model"
			var chunks []string
			out, err := Generate(context.Background(), cfg, ContextPackage{Fits: true, Sections: []ContextSection{{Text: "正文"}}}, "continue", func(s string) error { chunks = append(chunks, s); return nil })
			if err != nil {
				t.Fatal(err)
			}
			if out != "你好世界" || len(chunks) != 2 {
				t.Fatalf("wrong stream: %q %#v", out, chunks)
			}
		})
	}
}
func TestReviewAndImportGuards(t *testing.T) {
	p := NewProject("规则", false)
	p.Chapters[0].Text = "命运的齿轮开始转动。。\n相同的长句子重复在本章中需要检查是否有必要。\n相同的长句子重复在本章中需要检查是否有必要。"
	p.Banned = "命运的齿轮"
	r, _ := ReviewChapter(p, p.Chapters[0].ID)
	if len(r.Issues) != 3 {
		t.Fatalf("expected 3 issues got %d", len(r.Issues))
	}
	p.Entities = append(p.Entities, Entity{ID: ID(), Kind: "character", Versions: []Version{{StageID: p.Stages[0].ID}, {StageID: p.Stages[0].ID}}})
	if Validate(p) == nil {
		t.Fatal("duplicate stage versions accepted")
	}
}
func TestHTTPRejectsCrossOriginMutation(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	h := (&Server{Store: s}).Handler()
	r := httptest.NewRequest("POST", "http://127.0.0.1:48120/api/projects", strings.NewReader(`{"name":"unsafe"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("wrong status %d", w.Code)
	}
}
func TestDetectorContractAndConfigRedaction(t *testing.T) {
	p := NewProject("检测", false)
	p.Chapters[0].Text = strings.Repeat("中文小说片段。", 35)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["language"] != "zh" || payload["text"] != p.Chapters[0].Text {
			t.Error("wrong detection input")
		}
		fmt.Fprint(w, `{"score":0.3,"label":"参考结果","model":"test-detector","explanation":"测试协议，不是实际来源结论","language":"zh"}`)
	}))
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.DetectorURL = srv.URL
	cfg.APIKey = "test-secret"
	cfg.DetectorKey = "test-detector-secret"
	out, err := Detect(context.Background(), cfg, p, p.Chapters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Score == nil || *out.Score != 0.3 || out.Revision != p.Revision {
		t.Fatal("bad detector response")
	}
	safe := PublicConfig(cfg)
	if safe.APIKey != "" || safe.DetectorKey != "" || !safe.HasKey || !safe.HasDetectorKey {
		t.Fatal("config leaked keys")
	}
	p.Chapters[0].Text = "短句"
	if _, err = Detect(context.Background(), cfg, p, p.Chapters[0].ID); err == nil {
		t.Fatal("short input sent to detector")
	}
}
func TestExportImportAndDesktopToken(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	p, _ := s.Create("快照", true)
	handler := (&Server{Store: s, Token: "test-session"}).Handler()
	request := func(method, path string, body io.Reader) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:48120"+path, body)
		r.Header.Set("Origin", "http://wails.localhost")
		r.Header.Set("X-Novel-Token", "test-session")
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	export := request("GET", "/api/projects/"+p.ID+"/export?format=json", nil)
	if export.Code != 200 {
		t.Fatal(export.Body.String())
	}
	imported := request("POST", "/api/import", bytes.NewReader(export.Body.Bytes()))
	if imported.Code != 201 {
		t.Fatal(imported.Body.String())
	}
	var copy Project
	json.Unmarshal(imported.Body.Bytes(), &copy)
	if copy.ID == p.ID || copy.Chapters[0].Text != p.Chapters[0].Text {
		t.Fatal("snapshot import lost data or reused project ID")
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:48120/api/projects", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("desktop api accepted missing session token")
	}
	r = httptest.NewRequest("GET", "http://evil.example:48120/api/projects", nil)
	r.Header.Set("X-Novel-Token", "test-session")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("nonlocal Host accepted")
	}
}
func TestAIInspirationUsesActualSources(t *testing.T) {
	p := NewProject("", true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"message":{"content":"{\"queries\":[\"信封\",\"旧渡口\"]}"},"done":true}`)
	}))
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.Model = "test"
	cfg.BaseURL = srv.URL
	out, err := SearchWithAI(context.Background(), cfg, p, InspirationRequest{AIRequest: AIRequest{ChapterID: p.Chapters[0].ID}, Query: "找写信件悬念的片段"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Queries) != 2 || len(out.Results) == 0 {
		t.Fatal("AI keywords did not retrieve real paragraphs")
	}
	for _, r := range out.Results {
		if r.ChapterID != p.Chapters[0].ID || !strings.Contains(p.Chapters[0].Text, strings.TrimSuffix(r.Text, "…")) {
			t.Fatal("invented source paragraph")
		}
	}
}
