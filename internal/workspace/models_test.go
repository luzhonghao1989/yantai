package workspace

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelCapacityDiscovery(t *testing.T) {
	for _, provider := range []string{"ollama", "compatible", "lmstudio", "unknown"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/show":
					fmt.Fprint(w, `{"model_info":{"general.architecture":"qwen3","qwen3.context_length":262144}}`)
				case "/v1/models":
					if provider == "compatible" {
						fmt.Fprint(w, `{"data":[{"id":"test","context_window":4194304,"max_output_tokens":262144}]}`)
					} else {
						fmt.Fprint(w, `{"data":[{"id":"test"}]}`)
					}
				case "/api/v0/models/test":
					if provider == "lmstudio" {
						fmt.Fprint(w, `{"max_context_length":262144}`)
					} else {
						w.WriteHeader(404)
					}
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			cfg := DefaultConfig()
			cfg.Model = "test"
			cfg.BaseURL = server.URL
			if provider != "ollama" {
				cfg.Provider = "compatible"
				cfg.BaseURL += "/v1"
			}
			info := ReadModelInfo(context.Background(), cfg)
			switch provider {
			case "compatible":
				if info.ContextLimit != 4194304 || info.OutputLimit != 262144 {
					t.Fatal(info)
				}
			case "unknown":
				if info.ContextLimit != 0 || info.OutputLimit != 0 {
					t.Fatal("invented capacity", info)
				}
			default:
				if info.ContextLimit != 262144 {
					t.Fatal(info)
				}
			}
		})
	}
}
