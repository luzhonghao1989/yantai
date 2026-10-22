package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ModelInfo struct {
	Model        string `json:"model"`
	ContextLimit int    `json:"contextLimit,omitempty"`
	OutputLimit  int    `json:"outputLimit,omitempty"`
	Source       string `json:"source"`
}

func fetchModelJSON(ctx context.Context, cfg AIConfig, method, address string, body []byte) (map[string]any, error) {
	req, err := makeRequest(ctx, method, address, cfg.APIKey, body)
	if err != nil {
		return nil, err
	}
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, errors.New("服务未提供模型容量信息")
	}
	var data map[string]any
	err = json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&data)
	return data, err
}
func readLimit(data map[string]any, keys ...string) int {
	for _, key := range keys {
		if v, ok := data[key].(float64); ok && v > 0 && v < 9007199254740992 && v == float64(int(v)) {
			return int(v)
		}
	}
	return 0
}
func ReadModelInfo(ctx context.Context, cfg AIConfig) ModelInfo {
	out := ModelInfo{Model: cfg.Model, Source: "服务未提供容量信息，请手动填写"}
	if cfg.Model == "" {
		return out
	}
	if cfg.Provider == "ollama" {
		body, _ := json.Marshal(map[string]string{"model": cfg.Model})
		data, err := fetchModelJSON(ctx, cfg, "POST", endpoint(cfg, "/api/show"), body)
		if err != nil {
			return out
		}
		if info, ok := data["model_info"].(map[string]any); ok {
			architecture, _ := info["general.architecture"].(string)
			out.ContextLimit = readLimit(info, architecture+".context_length", "context_length")
			if out.ContextLimit == 0 {
				for key := range info {
					if strings.HasSuffix(key, ".context_length") {
						out.ContextLimit = readLimit(info, key)
						break
					}
				}
			}
		}
		if out.ContextLimit > 0 {
			out.Source = "Ollama 模型声明的上下文容量"
		}
		return out
	}
	// Some compatible servers expose capacity in /models; the standard may omit it.
	data, err := fetchModelJSON(ctx, cfg, "GET", endpoint(cfg, "/models"), nil)
	if err == nil {
		if entries, ok := data["data"].([]any); ok {
			for _, entry := range entries {
				if m, ok := entry.(map[string]any); ok && m["id"] == cfg.Model {
					out.ContextLimit = readLimit(m, "max_context_length", "context_length", "context_window")
					out.OutputLimit = readLimit(m, "max_output_tokens", "max_completion_tokens")
				}
			}
		}
	}
	// LM Studio's native metadata endpoint is queried only on loopback services.
	parsed, _ := url.Parse(cfg.BaseURL)
	if out.ContextLimit == 0 && parsed != nil && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1") {
		base := strings.TrimSuffix(strings.TrimRight(cfg.BaseURL, "/"), "/v1")
		if data, err := fetchModelJSON(ctx, cfg, "GET", base+"/api/v0/models/"+url.PathEscape(cfg.Model), nil); err == nil {
			out.ContextLimit = readLimit(data, "max_context_length", "context_length")
			out.OutputLimit = readLimit(data, "max_output_tokens")
		}
	}
	if out.ContextLimit > 0 || out.OutputLimit > 0 {
		out.Source = "服务返回的模型容量"
	}
	return out
}
func (s *Server) registerModelDiscovery(mux *http.ServeMux) {
	load := func(w http.ResponseWriter, r *http.Request) (AIConfig, error) {
		var cfg AIConfig
		if err := decode(w, r, &cfg); err != nil {
			return cfg, err
		}
		// Only reuse a stored secret for its original service, never send it to a new URL.
		old, err := s.Store.Config()
		if err != nil {
			return cfg, err
		}
		if cfg.APIKey == "" && cfg.HasKey && cfg.BaseURL == old.BaseURL && cfg.Provider == old.Provider {
			cfg.APIKey = old.APIKey
		}
		cfg.ContextLimit = 2
		cfg.OutputLimit = 1
		return cfg, ValidateConfig(cfg)
	}
	mux.HandleFunc("POST /api/models/discover", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := load(w, r)
		if err != nil {
			fail(w, err)
			return
		}
		models, err := TestModel(r.Context(), cfg)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]any{"models": models})
	})
	mux.HandleFunc("POST /api/models/info", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := load(w, r)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, ReadModelInfo(r.Context(), cfg))
	})
}
