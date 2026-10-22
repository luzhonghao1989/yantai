package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type Server struct {
	Store  *Store
	Assets fs.FS
	Token  string
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, err error) {
	status := 400
	if errors.Is(err, ErrConflict) {
		status = 409
	}
	reply(w, status, map[string]string{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 24<<20)).Decode(v)
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerAgent(mux)
	s.registerModelDiscovery(mux)
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Store.List()
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, v)
	})
	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
			Demo bool   `json:"demo"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		p, err := s.Store.Create(in.Name, in.Demo)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 201, p)
	})
	mux.HandleFunc("GET /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, p)
	})
	mux.HandleFunc("PUT /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		var p Project
		if err := decode(w, r, &p); err != nil {
			fail(w, err)
			return
		}
		if p.ID != r.PathValue("id") {
			fail(w, errors.New("项目 ID 不匹配"))
			return
		}
		v, err := s.Store.Save(p)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, v)
	})
	mux.HandleFunc("GET /api/projects/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Store.History(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, v)
	})
	mux.HandleFunc("POST /api/projects/{id}/restore", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Revision int64 `json:"revision"`
			Expected int64 `json:"expected"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		p, err := s.Store.Restore(r.PathValue("id"), in.Revision, in.Expected)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, p)
	})
	mux.HandleFunc("GET /api/projects/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		format := r.URL.Query().Get("format")
		extension := "json"
		var data []byte
		if format == "docx" || format == "epub" {
			extension = format
			if format == "docx" {
				data, err = ExportDOCX(p)
			} else {
				data, err = ExportEPUB(p)
			}
			if err != nil {
				fail(w, err)
				return
			}
		} else if format == "txt" || format == "md" {
			extension = format
			var b strings.Builder
			for _, v := range p.Volumes {
				if format == "md" {
					fmt.Fprintf(&b, "# %s\n\n", v.Name)
				} else {
					fmt.Fprintf(&b, "%s\n\n", v.Name)
				}
				for _, c := range p.Chapters {
					if c.VolumeID != v.ID {
						continue
					}
					if format == "md" {
						fmt.Fprintf(&b, "## %s\n\n%s\n\n", c.Title, c.Text)
					} else {
						fmt.Fprintf(&b, "%s\n\n%s\n\n", c.Title, c.Text)
					}
				}
			}
			data = []byte(b.String())
		} else {
			data, _ = json.MarshalIndent(p, "", "  ")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		if format == "docx" {
			w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		}
		if format == "epub" {
			w.Header().Set("Content-Type", "application/epub+zip")
		}
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(p.Name+"."+extension))
		w.Write(data)
	})
	mux.HandleFunc("POST /api/import", func(w http.ResponseWriter, r *http.Request) {
		var in Project
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		if err := Validate(in); err != nil {
			fail(w, err)
			return
		}
		p, err := s.Store.Create(in.Name+" · 导入", false)
		if err != nil {
			fail(w, err)
			return
		}
		in.ID = p.ID
		in.Name = p.Name
		in.Revision = p.Revision
		p, err = s.Store.Save(in)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 201, p)
	})
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := s.Store.Config()
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, PublicConfig(cfg))
	})
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var cfg AIConfig
		if err := decode(w, r, &cfg); err != nil {
			fail(w, err)
			return
		}
		cfg, err := s.Store.SaveConfig(cfg)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, PublicConfig(cfg))
	})
	mux.HandleFunc("POST /api/models/test", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := s.Store.Config()
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
	loadRequest := func(w http.ResponseWriter, r *http.Request) (Project, AIRequest, AIConfig, error) {
		var in AIRequest
		if err := decode(w, r, &in); err != nil {
			return Project{}, in, AIConfig{}, err
		}
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			return p, in, AIConfig{}, err
		}
		if p.Revision != in.Revision {
			return p, in, AIConfig{}, ErrConflict
		}
		cfg, err := s.Store.Config()
		return p, in, cfg, err
	}
	mux.HandleFunc("POST /api/projects/{id}/context", func(w http.ResponseWriter, r *http.Request) {
		p, in, cfg, err := loadRequest(w, r)
		if err != nil {
			fail(w, err)
			return
		}
		cp, err := BuildContext(p, in, cfg)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, cp)
	})
	mux.HandleFunc("POST /api/projects/{id}/generate", func(w http.ResponseWriter, r *http.Request) {
		p, in, cfg, err := loadRequest(w, r)
		if err != nil {
			fail(w, err)
			return
		}
		cp, err := BuildContext(p, in, cfg)
		if err != nil {
			fail(w, err)
			return
		}
		if !cp.Fits {
			fail(w, errors.New("上下文超出模型窗口，请调整后重试"))
			return
		}
		if cfg.Model == "" {
			fail(w, errors.New("请先配置模型名称"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, ok := w.(http.Flusher)
		if !ok {
			fail(w, errors.New("当前环境不支持流式输出"))
			return
		}
		send := func(kind string, data any) error {
			b, _ := json.Marshal(map[string]any{"type": kind, "data": data})
			_, err := fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
			return err
		}
		send("context", cp)
		text, err := Generate(r.Context(), cfg, cp, in.Mode, func(part string) error { return send("delta", part) })
		if err != nil {
			send("error", err.Error())
			return
		}
		send("done", map[string]any{"text": text, "revision": p.Revision, "model": cfg.Model, "id": ID()})
	})
	mux.HandleFunc("GET /api/projects/{id}/review", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		out, err := ReviewChapter(p, r.URL.Query().Get("chapter"))
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("POST /api/projects/{id}/detect", func(w http.ResponseWriter, r *http.Request) {
		p, in, cfg, err := loadRequest(w, r)
		if err != nil {
			fail(w, err)
			return
		}
		out, err := Detect(r.Context(), cfg, p, in.ChapterID)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("GET /api/projects/{id}/search", func(w http.ResponseWriter, r *http.Request) {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(query) > 1000 {
			fail(w, errors.New("搜索词过长"))
			return
		}
		if r.URL.Query().Get("scope") == "web" {
			cfg, err := s.Store.Config()
			if err != nil {
				fail(w, err)
				return
			}
			out, err := WebSearch(r.Context(), cfg.SearchKey, query)
			if err != nil {
				fail(w, err)
				return
			}
			reply(w, 200, out)
			return
		}
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		out := Search(p, query)
		sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
		if len(out) > 30 {
			out = out[:30]
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("POST /api/projects/{id}/ai-search", func(w http.ResponseWriter, r *http.Request) {
		var in InspirationRequest
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		if p.Revision != in.Revision {
			fail(w, ErrConflict)
			return
		}
		if strings.TrimSpace(in.Query) == "" || len(in.Query) > 1000 {
			fail(w, errors.New("请填写不超过 1000 字节的检索需求"))
			return
		}
		cfg, err := s.Store.Config()
		if err != nil {
			fail(w, err)
			return
		}
		out, err := SearchWithAI(r.Context(), cfg, p, in)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("POST /api/projects/{id}/style-reference", func(w http.ResponseWriter, r *http.Request) {
		var in InspirationRequest
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		if p.Revision != in.Revision {
			fail(w, ErrConflict)
			return
		}
		if strings.TrimSpace(in.Query) == "" || len(in.Query) > 1000 {
			fail(w, errors.New("请填写不超过 1000 字节的检索需求"))
			return
		}
		cfg, err := s.Store.Config()
		if err != nil {
			fail(w, err)
			return
		}
		out, err := StyleReference(r.Context(), cfg, p, in)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("POST /api/projects/{id}/semantic-search", func(w http.ResponseWriter, r *http.Request) {
		var in InspirationRequest
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		if p.Revision != in.Revision {
			fail(w, ErrConflict)
			return
		}
		if strings.TrimSpace(in.Query) == "" || len(in.Query) > 1000 {
			fail(w, errors.New("请填写不超过 1000 字节的检索需求"))
			return
		}
		cfg, err := s.Store.Config()
		if err != nil {
			fail(w, err)
			return
		}
		out, err := s.Store.SemanticSearch(r.Context(), cfg, p, in.Query, in.ChapterID)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, out)
	})
	if s.Assets != nil {
		files := http.FileServer(http.FS(s.Assets))
		mux.Handle("/", files)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "请先构建前端：cd frontend && npm run build")
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			reply(w, 403, map[string]string{"error": "仅允许本机访问"})
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			origin := r.Header.Get("Origin")
			if origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host && !(s.Token != "" && (origin == "wails://wails" || origin == "http://wails.localhost" || origin == "https://wails.localhost")) {
				reply(w, 403, map[string]string{"error": "不允许跨来源请求"})
				return
			}
			if s.Token != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Novel-Token")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
				if r.Method == "OPTIONS" {
					w.WriteHeader(204)
					return
				}
				if r.Header.Get("X-Novel-Token") != s.Token {
					reply(w, 403, map[string]string{"error": "本地会话令牌无效"})
					return
				}
			}
			if r.Method != "GET" && r.Header.Get("Content-Type") != "application/json" {
				reply(w, 415, map[string]string{"error": "请求必须是 JSON"})
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func RevisionFromString(value string) int64 { n, _ := strconv.ParseInt(value, 10, 64); return n }
