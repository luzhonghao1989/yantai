package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestComfyWorkflowValidationAndSeed(t *testing.T) {
	c := ComfyConfig{BaseURL: "http://127.0.0.1:8188", PositiveNode: "6", NegativeNode: "7", OutputNode: "9", Workflow: `{"6":{"class_type":"CLIPTextEncode","inputs":{"text":"old"}},"7":{"class_type":"CLIPTextEncode","inputs":{"text":"negative"}},"3":{"class_type":"KSampler","inputs":{"seed":123,"steps":8}},"9":{"class_type":"SaveImage","inputs":{"filename_prefix":"Yantai"}}}`}
	seed := int64(42)
	nodes, actual, err := prepareComfyWorkflow(c, comfyStart{Prompt: "古宅庭院", Negative: "模糊", Seed: &seed})
	if err != nil || actual != 42 || nodes["6"].Inputs["text"] != "古宅庭院" || nodes["7"].Inputs["text"] != "模糊" || nodes["3"].Inputs["seed"] != seed || nodes["3"].Inputs["steps"] != float64(8) {
		t.Fatalf("workflow binding failed: %v %#v", err, nodes)
	}
	if !strings.Contains(c.Workflow, `"text":"old"`) {
		t.Fatal("configuration mutated")
	}
	for _, change := range []func(*ComfyConfig){func(c *ComfyConfig) { c.Workflow = `{"nodes":[],"links":[]}` }, func(c *ComfyConfig) { c.PositiveNode = "missing" }, func(c *ComfyConfig) { c.OutputNode = "missing" }, func(c *ComfyConfig) { c.BaseURL = "file:///tmp" }, func(c *ComfyConfig) { c.NegativeNode = "6" }} {
		bad := c
		change(&bad)
		if validateComfyConfig(bad, true) == nil {
			t.Fatalf("invalid config accepted: %#v", bad)
		}
	}
	if _, _, err = prepareComfyWorkflow(c, comfyStart{}); err == nil {
		t.Fatal("empty prompt accepted")
	}
}

func TestComfyQueuePollImageAndSecrets(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	polls := 0
	viewed := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Error("missing ComfyUI auth")
		}
		switch {
		case r.URL.Path == "/prompt":
			var request struct {
				Prompt map[string]comfyNode `json:"prompt"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Prompt["text"].Inputs["text"] != "古宅平面图" {
				t.Error("missing live prompt")
			}
			reply(w, 200, map[string]any{"prompt_id": "upstream-fixture-id", "node_errors": map[string]any{}})
		case r.URL.Path == "/history/upstream-fixture-id":
			polls++
			if polls == 1 {
				reply(w, 200, map[string]any{})
				return
			}
			reply(w, 200, map[string]any{"upstream-fixture-id": map[string]any{"status": map[string]any{"completed": true, "status_str": "success"}, "outputs": map[string]any{"save": map[string]any{"images": []map[string]string{{"filename": "古宅 & 1.png", "subfolder": "Yantai/test", "type": "output"}}}}}})
		case r.URL.Path == "/view":
			if r.URL.Query().Get("filename") != "古宅 & 1.png" || r.URL.Query().Get("subfolder") != "Yantai/test" {
				t.Error("image URL not encoded")
			}
			viewed = true
			w.Write(pngData.Bytes())
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Comfy = ComfyConfig{BaseURL: upstream.URL, APIKey: "fixture-secret", PositiveNode: "text", OutputNode: "save", Workflow: `{"text":{"class_type":"CLIPTextEncode","inputs":{"text":""}},"save":{"class_type":"SaveImage","inputs":{}}}`}
	cfg, err = s.SaveConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	public := PublicConfig(cfg)
	if public.Comfy.APIKey != "" || !public.Comfy.HasKey {
		t.Fatal("secret exposed")
	}
	if _, err = s.SaveConfig(public); err != nil {
		t.Fatal(err)
	}
	cfg, _ = s.Config()
	if cfg.Comfy.APIKey != "fixture-secret" {
		t.Fatal("stored secret lost")
	}
	id, _, err := s.startComfy(context.Background(), comfyStart{Prompt: "古宅平面图"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.pollComfy(context.Background(), id)
	if err != nil || first.Status != "waiting" {
		t.Fatalf("waiting: %v %#v", err, first)
	}
	// Changing global settings must not redirect an already-submitted job.
	cfg.Comfy.BaseURL = "http://127.0.0.1:1"
	if _, err = s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	out, err := s.pollComfy(context.Background(), id)
	if err != nil || out.Status != "completed" || len(out.Images) != 1 || !viewed {
		t.Fatalf("completion: %v %#v", err, out)
	}
	if out.Images[0].MIME != "image/png" || out.Images[0].SourceRunID != "comfy-"+id {
		t.Fatal("image metadata missing")
	}
	p, err := s.Create("ComfyUI image persistence", false)
	if err != nil {
		t.Fatal(err)
	}
	p.Images = out.Images
	p, err = s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.Load(p.ID)
	if err != nil || p.Images[0].SHA == "" || p.Images[0].Data != "" {
		t.Fatal("image not stored in content-addressed library")
	}
	if _, err = s.pollComfy(context.Background(), ID()); err == nil {
		t.Fatal("unknown job accepted")
	}
	handler := (&Server{Store: s, Token: "app-token"}).Handler()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "http://127.0.0.1:48120/api/comfyui/jobs", strings.NewReader(`{"prompt":"test"}`)))
	if w.Code != 403 {
		t.Fatalf("route lacks session authentication: %d", w.Code)
	}
}
