package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

func embedTexts(ctx context.Context, cfg AIConfig, texts []string) ([][]float64, error) {
	if strings.TrimSpace(cfg.EmbeddingModel) == "" {
		return nil, errors.New("请在模型设置中配置向量模型，或使用本地关键词检索")
	}
	path := "/embeddings"
	payload := map[string]any{"model": cfg.EmbeddingModel, "input": texts}
	if cfg.Provider == "ollama" {
		path = "/api/embed"
		payload["truncate"] = false
	}
	body, _ := json.Marshal(payload)
	req, err := makeRequest(ctx, "POST", endpoint(cfg, path), cfg.APIKey, body)
	if err != nil {
		return nil, err
	}
	res, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return nil, errors.New("向量模型连接失败或超时")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("向量服务返回 HTTP %d，请检查模型是否支持嵌入接口", res.StatusCode)
	}
	var data struct {
		Embeddings [][]float64 `json:"embeddings"`
		Data       []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 32<<20)).Decode(&data); err != nil {
		return nil, errors.New("向量服务返回无效数据")
	}
	vectors := data.Embeddings
	if cfg.Provider == "compatible" {
		vectors = make([][]float64, len(texts))
		for _, row := range data.Data {
			if row.Index < 0 || row.Index >= len(vectors) || vectors[row.Index] != nil {
				return nil, errors.New("向量返回索引无效")
			}
			vectors[row.Index] = row.Embedding
		}
	}
	if len(vectors) != len(texts) {
		return nil, errors.New("向量返回数量与输入不匹配")
	}
	dimensions := 0
	for _, v := range vectors {
		if len(v) == 0 {
			return nil, errors.New("模型返回空向量")
		}
		if dimensions == 0 {
			dimensions = len(v)
		}
		if len(v) != dimensions {
			return nil, errors.New("向量维度不一致")
		}
		norm := 0.0
		for _, x := range v {
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return nil, errors.New("向量含无效数值")
			}
			norm += x * x
		}
		if norm == 0 || math.IsInf(norm, 0) {
			return nil, errors.New("模型返回不可用向量")
		}
	}
	return vectors, nil
}
func cosine(a, b []float64) float64 {
	if len(a) != len(b) {
		return -1
	}
	dot, aa, bb := 0.0, 0.0, 0.0
	for i, x := range a {
		dot += x * b[i]
		aa += x * x
		bb += b[i] * b[i]
	}
	return dot / math.Sqrt(aa*bb)
}
func semanticChunks(p Project, chapterID string) ([]SearchResult, error) {
	out := []SearchResult{}
	add := func(title, text, source, url, id string) {
		for n, para := range strings.Split(text, "\n") {
			r := []rune(strings.TrimSpace(para))
			for start := 0; start < len(r); start += 500 {
				end := start + 500
				if end > len(r) {
					end = len(r)
				}
				out = append(out, SearchResult{Title: title, Text: string(r[start:end]), Source: fmt.Sprintf("%s · 第 %d 段", source, n+1), URL: url, ChapterID: id})
			}
		}
	}
	current := -1
	maxStage := -1
	orders := map[string]int{}
	for i, s := range p.Stages {
		orders[s.ID] = i
	}
	if chapterID != "" {
		for i, c := range p.Chapters {
			if c.ID == chapterID {
				current = i
				maxStage = orders[c.StageID]
				break
			}
		}
		if current < 0 {
			return nil, errors.New("当前章节不存在")
		}
	}
	for i, c := range p.Chapters {
		if current >= 0 && (i > current || orders[c.StageID] > maxStage) {
			continue
		}
		add(c.Title, c.Text, "本项目正文", "", c.ID)
	}
	for _, s := range p.Sources {
		add(s.Title, s.Text, "导入素材", s.URL, "")
	}
	return out, nil
}
func (s *Store) SemanticSearch(ctx context.Context, cfg AIConfig, p Project, query, chapterID string) ([]SearchResult, error) {
	chunks, err := semanticChunks(p, chapterID)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return []SearchResult{}, nil
	}
	queryVectors, err := embedTexts(ctx, cfg, []string{query})
	if err != nil {
		return nil, err
	}
	s.embeddingMu.Lock()
	defer s.embeddingMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.embeddings == nil {
		s.embeddings = map[string][]float64{}
	}
	keys := make([]string, len(chunks))
	vectors := make([][]float64, len(chunks))
	missing := []int{}
	for i, c := range chunks {
		keys[i] = fmt.Sprintf("%x", sha256.Sum256([]byte(p.ID+"\x00"+cfg.Provider+"\x00"+cfg.BaseURL+"\x00"+cfg.EmbeddingModel+"\x00"+c.Text)))
		if v, ok := s.embeddings[keys[i]]; ok && len(v) == len(queryVectors[0]) {
			vectors[i] = v
		} else {
			missing = append(missing, i)
		}
	}
	for start := 0; start < len(missing); start += 24 {
		end := start + 24
		if end > len(missing) {
			end = len(missing)
		}
		texts := []string{}
		for _, i := range missing[start:end] {
			texts = append(texts, chunks[i].Text)
		}
		batch, err := embedTexts(ctx, cfg, texts)
		if err != nil {
			return nil, err
		}
		for n, i := range missing[start:end] {
			if len(batch[n]) != len(queryVectors[0]) {
				return nil, errors.New("查询与正文向量维度不一致")
			}
			vectors[i] = batch[n]
			s.embeddings[keys[i]] = batch[n]
		}
	}
	for i := range chunks {
		chunks[i].Score = int(math.Round(cosine(queryVectors[0], vectors[i]) * 1000))
		chunks[i].Source += " · 语义相似度"
	}
	sort.SliceStable(chunks, func(i, j int) bool { return chunks[i].Score > chunks[j].Score })
	if len(chunks) > 20 {
		chunks = chunks[:20]
	}
	return chunks, nil
}
