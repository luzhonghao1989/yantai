package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type InspirationRequest struct {
	AIRequest
	Query string `json:"query"`
	Scope string `json:"scope"`
}
type InspirationResult struct {
	Queries []string       `json:"queries"`
	Results []SearchResult `json:"results"`
}

func SearchWithAI(ctx context.Context, cfg AIConfig, p Project, in InspirationRequest) (InspirationResult, error) {
	req := in.AIRequest
	req.Mode = "search"
	req.Instruction = "把以下需求转成适合检索相关写作片段的关键词或短语，不生成片段：" + in.Query
	cp, err := BuildContext(p, req, cfg)
	if err != nil {
		return InspirationResult{}, err
	}
	text, err := Generate(ctx, cfg, cp, "search", func(string) error { return nil })
	if err != nil {
		return InspirationResult{}, err
	}
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return InspirationResult{}, errors.New("模型未返回检索词 JSON，请换模型或使用普通关键词检索")
	}
	var out InspirationResult
	if err = json.Unmarshal([]byte(text[start:end+1]), &out); err != nil {
		return out, errors.New("模型返回的检索词格式无效")
	}
	if len(out.Queries) == 0 {
		return out, errors.New("模型没有给出检索词")
	}
	if len(out.Queries) > 3 {
		out.Queries = out.Queries[:3]
	}
	out.Results = []SearchResult{}
	seen := map[string]bool{}
	for i, q := range out.Queries {
		q = strings.TrimSpace(truncate(q, 100))
		out.Queries[i] = q
		if q == "" {
			continue
		}
		var found []SearchResult
		if in.Scope == "web" {
			found, err = WebSearch(ctx, cfg.SearchKey, q)
			if err != nil {
				return out, err
			}
		} else {
			found = Search(p, q)
		}
		if in.Mode == "style" && len(found) > 3 {
			found = found[:3]
		}
		for _, r := range found {
			key := r.URL + "|" + r.Title + "|" + r.Text
			if !seen[key] {
				seen[key] = true
				out.Results = append(out.Results, r)
			}
		}
	}
	sort.SliceStable(out.Results, func(i, j int) bool { return out.Results[i].Score > out.Results[j].Score })
	if len(out.Results) > 30 {
		out.Results = out.Results[:30]
	}
	return out, nil
}

type StyleReferenceResult struct {
	InspirationResult
	Analysis string `json:"analysis"`
}

func StyleReference(ctx context.Context, cfg AIConfig, p Project, in InspirationRequest) (StyleReferenceResult, error) {
	in.Scope = "web"
	in.Mode = "style"
	in.Query = "寻找小说中相似桥段的文风与场景描写参考：" + in.Query
	found, err := SearchWithAI(ctx, cfg, p, in)
	if err != nil {
		return StyleReferenceResult{}, err
	}
	return synthesizeStyle(ctx, cfg, p, in, found)
}

func synthesizeStyle(ctx context.Context, cfg AIConfig, p Project, in InspirationRequest, found InspirationResult) (StyleReferenceResult, error) {
	if len(found.Results) == 0 {
		return StyleReferenceResult{}, errors.New("没有检索到网页参考，请换一个具体场景或关键词")
	}
	if len(found.Results) > 8 {
		found.Results = found.Results[:8]
	}
	var refs strings.Builder
	for i, r := range found.Results {
		fmt.Fprintf(&refs, "\n[%d] %s\n来源：%s\n网页摘要（不是全文）：%s\n", i+1, r.Title, r.URL, truncate(r.Text, 400))
	}
	req := in.AIRequest
	req.Mode = "style"
	req.Instruction = in.Query + "\n参考以下实际检索结果，分析文风和描写技巧，并结合本章给出原创写法。\n" + refs.String()
	cp, err := BuildContext(p, req, cfg)
	if err != nil {
		return StyleReferenceResult{}, err
	}
	analysis, err := Generate(ctx, cfg, cp, "style", func(string) error { return nil })
	return StyleReferenceResult{InspirationResult: found, Analysis: analysis}, err
}
