package workspace

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type ComfyConfig struct {
	BaseURL       string `json:"baseUrl"`
	APIKey        string `json:"apiKey"`
	HasKey        bool   `json:"hasKey"`
	Workflow      string `json:"workflow"`
	PositiveNode  string `json:"positiveNode"`
	PositiveInput string `json:"positiveInput"`
	NegativeNode  string `json:"negativeNode"`
	NegativeInput string `json:"negativeInput"`
	OutputNode    string `json:"outputNode"`
}
type comfyNode struct {
	ClassType string          `json:"class_type"`
	Inputs    map[string]any  `json:"inputs"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}
type comfyJob struct {
	Config   ComfyConfig
	Created  time.Time
	Seed     int64
	PromptID string
}
type comfyStart struct {
	Prompt   string `json:"prompt"`
	Negative string `json:"negative"`
	Seed     *int64 `json:"seed"`
}
type comfyResult struct {
	Status string         `json:"status"`
	Images []ProjectImage `json:"images,omitempty"`
	Seed   int64          `json:"seed"`
}

func comfyWorkflow(c ComfyConfig) (map[string]comfyNode, error) {
	var nodes map[string]comfyNode
	if len(c.Workflow) > 2<<20 {
		return nil, errors.New("ComfyUI 工作流最多 2 MiB")
	}
	if err := json.Unmarshal([]byte(c.Workflow), &nodes); err != nil || len(nodes) == 0 {
		return nil, errors.New("请导入 ComfyUI 的 API 格式工作流 JSON（导出 API），不是界面工作流")
	}
	for _, node := range nodes {
		if node.ClassType == "" || node.Inputs == nil {
			return nil, errors.New("工作流不是有效的 API 格式，请在 ComfyUI 导出 API JSON")
		}
	}
	return nodes, nil
}
func validateComfyConfig(c ComfyConfig, required bool) error {
	if !required && strings.TrimSpace(c.BaseURL) == "" && strings.TrimSpace(c.Workflow) == "" {
		return nil
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("ComfyUI 地址须为 HTTP/HTTPS 服务地址，不含账号、查询参数或片段")
	}
	if c.Workflow == "" && !required {
		return nil
	}
	nodes, err := comfyWorkflow(c)
	if err != nil {
		return err
	}
	field := c.PositiveInput
	if field == "" {
		field = "text"
	}
	if value, ok := nodes[c.PositiveNode].Inputs[field]; !ok {
		return errors.New("请指定工作流中有效的正向提示词节点和输入字段")
	} else if _, ok := value.(string); !ok {
		return errors.New("正向提示词输入必须是文字字段，不能是节点连线")
	}
	if c.NegativeNode != "" {
		if c.NegativeNode == c.PositiveNode && (c.NegativeInput == "" || c.NegativeInput == field) {
			return errors.New("正向和负向提示词不能使用同一个字段")
		}
		nf := c.NegativeInput
		if nf == "" {
			nf = "text"
		}
		if _, ok := nodes[c.NegativeNode].Inputs[nf].(string); !ok {
			return errors.New("负向提示词节点或文字字段无效")
		}
	}
	if c.OutputNode != "" {
		if _, ok := nodes[c.OutputNode]; !ok {
			return errors.New("图片输出节点不存在")
		}
	}
	return nil
}
func prepareComfyWorkflow(c ComfyConfig, in comfyStart) (map[string]comfyNode, int64, error) {
	if err := validateComfyConfig(c, true); err != nil {
		return nil, 0, err
	}
	if strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > 60000 || len(in.Negative) > 30000 {
		return nil, 0, errors.New("请填写生图提示词，正向最多 60000 字节、负向最多 30000 字节")
	}
	nodes, _ := comfyWorkflow(c)
	seed := int64(0)
	if in.Seed != nil {
		seed = *in.Seed
		if seed < 0 || seed > 9007199254740991 {
			return nil, 0, errors.New("种子须为 0 至 9007199254740991 的整数，留空随机")
		}
	} else {
		n, err := rand.Int(rand.Reader, big.NewInt(9007199254740992))
		if err != nil {
			return nil, 0, err
		}
		seed = n.Int64()
	}
	field := c.PositiveInput
	if field == "" {
		field = "text"
	}
	nodes[c.PositiveNode].Inputs[field] = in.Prompt
	if c.NegativeNode != "" {
		field = c.NegativeInput
		if field == "" {
			field = "text"
		}
		nodes[c.NegativeNode].Inputs[field] = in.Negative
	}
	for _, node := range nodes {
		for _, key := range []string{"seed", "noise_seed"} {
			if v, ok := node.Inputs[key].(float64); ok && !math.IsNaN(v) {
				node.Inputs[key] = seed
			}
		}
	}
	return nodes, seed, nil
}
func comfyCall(ctx context.Context, c ComfyConfig, method, path string, body any) ([]byte, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := makeRequest(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, c.APIKey, payload)
	if err != nil {
		return nil, err
	}
	// Credentials only go to the configured server, never through an HTTP redirect.
	client := &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("ComfyUI 连接未完成，请检查服务地址、运行状态或取消状态")
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, (10<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 10<<20 {
		return nil, errors.New("ComfyUI 返回内容过大")
	}
	if res.StatusCode >= 300 {
		// Do not echo arbitrary remote bodies, which can include credentials or node inputs.
		return nil, fmt.Errorf("ComfyUI 返回 HTTP %d，请检查连接、工作流节点和模型文件；详细原因可在 ComfyUI 查看", res.StatusCode)
	}
	return data, nil
}
func (s *Store) startComfy(ctx context.Context, in comfyStart) (string, int64, error) {
	cfg, err := s.Config()
	if err != nil {
		return "", 0, err
	}
	nodes, seed, err := prepareComfyWorkflow(cfg.Comfy, in)
	if err != nil {
		return "", 0, err
	}
	data, err := comfyCall(ctx, cfg.Comfy, "POST", "/prompt", map[string]any{"prompt": nodes, "client_id": "Yantai-" + ID()})
	if err != nil {
		return "", 0, err
	}
	var response struct {
		ID         string                     `json:"prompt_id"`
		Error      json.RawMessage            `json:"error"`
		NodeErrors map[string]json.RawMessage `json:"node_errors"`
	}
	if json.Unmarshal(data, &response) != nil || response.ID == "" || (len(response.Error) > 0 && string(response.Error) != "null") || len(response.NodeErrors) > 0 {
		return "", 0, errors.New("ComfyUI 未接受工作流，请检查缺失节点、模型及参数；详细原因可在 ComfyUI 查看")
	}
	jobID := ID()
	// Local opaque ID prevents arbitrary history lookups and pins config for this job.
	cfg.Comfy.Workflow = ""
	s.comfyMu.Lock()
	defer s.comfyMu.Unlock()
	if s.comfyJobs == nil {
		s.comfyJobs = map[string]comfyJob{}
	}
	for key, job := range s.comfyJobs {
		if time.Since(job.Created) > time.Hour {
			delete(s.comfyJobs, key)
		}
	}
	// Keep upstream prompt ID in an internal field without exposing endpoint credentials.
	s.comfyJobs[jobID] = comfyJob{Config: cfg.Comfy, Created: time.Now(), Seed: seed, PromptID: response.ID}
	return jobID, seed, nil
}
func (s *Store) pollComfy(ctx context.Context, id string) (comfyResult, error) {
	s.comfyMu.Lock()
	job := s.comfyJobs[id]
	promptID := job.PromptID
	s.comfyMu.Unlock()
	if promptID == "" || time.Since(job.Created) > time.Hour {
		return comfyResult{}, errors.New("生图任务不存在或已过期，请到 ComfyUI 查看历史结果")
	}
	data, err := comfyCall(ctx, job.Config, "GET", "/history/"+url.PathEscape(promptID), nil)
	if err != nil {
		return comfyResult{}, err
	}
	type imageFile struct {
		Filename  string `json:"filename"`
		Subfolder string `json:"subfolder"`
		Type      string `json:"type"`
	}
	var history map[string]struct {
		Outputs map[string]struct {
			Images []imageFile `json:"images"`
		} `json:"outputs"`
		Status struct {
			Completed bool   `json:"completed"`
			Status    string `json:"status_str"`
		} `json:"status"`
	}
	if json.Unmarshal(data, &history) != nil {
		return comfyResult{}, errors.New("ComfyUI 历史响应格式无效")
	}
	record, ok := history[promptID]
	out := comfyResult{Status: "waiting", Seed: job.Seed}
	if !ok {
		return out, nil
	}
	if record.Status.Status == "error" {
		return out, errors.New("ComfyUI 工作流执行失败或被中断，请在 ComfyUI 查看节点报错")
	}
	if !record.Status.Completed && record.Status.Status != "success" {
		return out, nil
	}
	keys := make([]string, 0, len(record.Outputs))
	for key := range record.Outputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var total int
	for _, key := range keys {
		if job.Config.OutputNode != "" && key != job.Config.OutputNode {
			continue
		}
		for _, file := range record.Outputs[key].Images {
			if len(out.Images) >= 8 {
				return out, errors.New("单次生图最多接收 8 张，请在工作流减少批量")
			}
			if file.Type != "output" && file.Type != "temp" {
				continue
			}
			q := url.Values{"filename": {file.Filename}, "subfolder": {file.Subfolder}, "type": {file.Type}}
			img, err := comfyCall(ctx, job.Config, "GET", "/view?"+q.Encode(), nil)
			if err != nil {
				return out, err
			}
			if len(img) > 8<<20 {
				return out, errors.New("生成图片超过单张 8 MiB，请在工作流降低分辨率")
			}
			encoded := "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(img)
			_, mime, err := decodeProjectImage(encoded)
			if err != nil || mime == "image/svg+xml" {
				return out, errors.New("ComfyUI 输出须为 PNG、JPEG 或 GIF 位图，请检查保存节点")
			}
			total += len(img)
			if total > 32<<20 {
				return out, errors.New("单次生图图片总量超过 32 MiB")
			}
			extension := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif"}[mime]
			out.Images = append(out.Images, ProjectImage{ID: ID(), Name: fmt.Sprintf("ComfyUI-%d%s", len(out.Images)+1, extension), Kind: "scene", Note: fmt.Sprintf("ComfyUI 生成；种子 %d", job.Seed), SourceRunID: "comfy-" + id, CreatedAt: Now(), MIME: mime, Size: int64(len(img)), Data: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(img)})
		}
	}
	if len(out.Images) == 0 {
		return out, errors.New("工作流已完成但没有可读取的图片，请检查输出节点，使用 SaveImage 或 PreviewImage 输出位图")
	}
	out.Status = "completed"
	return out, nil
}
func (s *Server) registerComfy(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/comfyui/test", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := s.Store.Config()
		if err != nil {
			fail(w, err)
			return
		}
		if err = validateComfyConfig(cfg.Comfy, true); err != nil {
			fail(w, err)
			return
		}
		data, err := comfyCall(r.Context(), cfg.Comfy, "GET", "/system_stats", nil)
		if err != nil {
			fail(w, err)
			return
		}
		var stats map[string]json.RawMessage
		if json.Unmarshal(data, &stats) != nil || stats["system"] == nil {
			fail(w, errors.New("该地址未返回 ComfyUI 系统信息，请确认服务地址"))
			return
		}
		reply(w, 200, map[string]string{"message": "ComfyUI 连接成功；工作流格式和提示词映射有效。模型及自定义节点在提交时由 ComfyUI 校验。"})
	})
	mux.HandleFunc("POST /api/comfyui/jobs", func(w http.ResponseWriter, r *http.Request) {
		var in comfyStart
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		id, seed, err := s.Store.startComfy(r.Context(), in)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 202, map[string]any{"id": id, "seed": seed})
	})
	mux.HandleFunc("GET /api/comfyui/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !validID.MatchString(id) {
			fail(w, errors.New("生图任务 ID 无效"))
			return
		}
		out, err := s.Store.pollComfy(r.Context(), id)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, out)
	})
}
