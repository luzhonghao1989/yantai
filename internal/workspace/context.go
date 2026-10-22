package workspace

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ContextSection struct {
	Name   string `json:"name"`
	Text   string `json:"text"`
	Source string `json:"source"`
}
type ContextPackage struct {
	Revision        int64            `json:"revision"`
	ChapterID       string           `json:"chapterId"`
	StageID         string           `json:"stageId"`
	Sections        []ContextSection `json:"sections"`
	EstimatedTokens int              `json:"estimatedTokens"`
	Budget          int              `json:"budget"`
	Fits            bool             `json:"fits"`
}
type AIRequest struct {
	ChapterID   string `json:"chapterId"`
	Revision    int64  `json:"revision"`
	Mode        string `json:"mode"`
	Instruction string `json:"instruction"`
	Selection   string `json:"selection"`
}

func BuildContext(p Project, req AIRequest, cfg AIConfig) (ContextPackage, error) {
	c, err := FindChapter(p, req.ChapterID)
	if err != nil {
		return ContextPackage{}, err
	}
	out := ContextPackage{Revision: p.Revision, ChapterID: c.ID, StageID: c.StageID, Sections: []ContextSection{}, Budget: cfg.ContextLimit - cfg.OutputLimit}
	add := func(name, text, source string) {
		if strings.TrimSpace(text) != "" {
			out.Sections = append(out.Sections, ContextSection{name, text, source})
		}
	}
	add("任务", fmt.Sprintf("模式：%s\n要求：%s", req.Mode, req.Instruction), "本次请求")
	add("世界规则", p.Rules, "项目规则")
	add("全书大纲与未来规划", p.Outline, "仅用于方向约束，未来事件不代表已发生")
	for _, v := range p.Volumes {
		if v.ID == c.VolumeID {
			add("卷纲", v.Outline, v.Name)
		}
	}
	for _, s := range p.Stages {
		if s.ID == c.StageID {
			add("当前阶段细纲", s.Outline, s.Name)
		}
	}
	add("章纲", c.Outline, c.Title)
	add("前情提要", c.Summary, c.Title)
	for _, line := range p.Storylines {
		for _, b := range line.Beats {
			if b.ChapterID == c.ID {
				add("故事线 · "+line.Name, b.Title+"\n状态："+b.Status+"\n"+b.Detail, c.Title)
			}
		}
	}
	for _, e := range Resolve(p, c) {
		text := e.Entity.Base
		for _, field := range ProfileFields {
			if value := e.Entity.Profile[field.Key]; value != "" {
				text += "\n" + field.Label + "：" + value
			}
		}
		if e.Version != nil {
			v := e.Version
			text += fmt.Sprintf("\n性格或特征：%s\n喜好：%s\n状态：%s\n已知信息：%s\n备注：%s", v.Traits, v.Likes, v.State, v.Knowledge, v.Note)
		} else {
			text += "\n当前阶段无有效版本，状态未知；不得自行补全为事实。"
		}
		add("设定 · "+e.Entity.Name, text, "有效阶段版本 "+c.StageID)
	}
	// Only summaries from preceding chapters at or before the story stage are eligible.
	stageOrder := map[string]int{}
	for i, s := range p.Stages {
		stageOrder[s.ID] = i
	}
	for i, ch := range p.Chapters {
		if ch.ID == c.ID {
			for j := i - 1; j >= 0; j-- {
				prev := p.Chapters[j]
				if stageOrder[prev.StageID] <= stageOrder[c.StageID] && prev.Summary != "" {
					add("前章提要", prev.Summary, prev.Title)
					break
				}
			}
			break
		}
	}
	add("文风要求", p.Style, "项目文风")
	add("避免的表达", p.Banned, "作者自定义")
	add("当前正文", c.Text, c.Title)
	add("选中文本", req.Selection, "仅修改此处时保留其余正文")
	total := 600
	for _, s := range out.Sections {
		total += utf8.RuneCountInString(s.Name+s.Text+s.Source) * 2
	}
	out.EstimatedTokens = total
	out.Fits = out.Budget > 0 && total <= out.Budget
	return out, nil
}
func (c ContextPackage) Prompt() string {
	var b strings.Builder
	for _, s := range c.Sections {
		fmt.Fprintf(&b, "\n## %s\n来源：%s\n%s\n", s.Name, s.Source, s.Text)
	}
	return b.String()
}

type Issue struct {
	Kind      string `json:"kind"`
	Paragraph int    `json:"paragraph"`
	Quote     string `json:"quote"`
	Message   string `json:"message"`
}
type Review struct {
	Revision      int64   `json:"revision"`
	ChapterID     string  `json:"chapterId"`
	Words         int     `json:"words"`
	Paragraphs    int     `json:"paragraphs"`
	AverageLength int     `json:"averageLength"`
	Issues        []Issue `json:"issues"`
	AIRecords     int     `json:"aiRecords"`
}

func ReviewChapter(p Project, id string) (Review, error) {
	c, err := FindChapter(p, id)
	if err != nil {
		return Review{}, err
	}
	out := Review{Revision: p.Revision, ChapterID: c.ID, Words: WordCount(c.Text), Issues: []Issue{}}
	seen := map[string]int{}
	paragraphs := strings.Split(c.Text, "\n")
	for i, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		out.Issues = append(out.Issues, spellingIssues(para, i+1)...)
		out.Paragraphs++
		n := WordCount(para)
		if n > 350 {
			out.Issues = append(out.Issues, Issue{"长段落", i + 1, truncate(para, 80), "本段超过 350 字，可检查动作与信息是否适合拆分。"})
		}
		if prev, ok := seen[para]; ok && n >= 15 {
			out.Issues = append(out.Issues, Issue{"章内重复", i + 1, truncate(para, 80), fmt.Sprintf("与第 %d 段完全相同。", prev)})
		}
		seen[para] = i + 1
		for _, term := range strings.Split(p.Banned, "\n") {
			term = strings.TrimSpace(term)
			if term != "" && strings.Contains(para, term) {
				out.Issues = append(out.Issues, Issue{"文风规则", i + 1, truncate(para, 80), "出现作者标记的表达：“" + term + "”。"})
			}
		}
		if strings.Contains(para, "。。") || strings.Contains(para, "，，") || strings.Contains(para, "！！") {
			out.Issues = append(out.Issues, Issue{"标点", i + 1, truncate(para, 80), "检查重复标点是否有意使用。"})
		}
		if n >= 25 {
			for _, other := range p.Chapters {
				if other.ID != c.ID && strings.Contains(other.Text, para) {
					out.Issues = append(out.Issues, Issue{"跨章重复", i + 1, truncate(para, 80), "在“" + other.Title + "”中也出现相同段落。"})
					break
				}
			}
		}
	}
	if out.Paragraphs > 0 {
		out.AverageLength = out.Words / out.Paragraphs
	}
	for _, r := range p.AIRuns {
		if r.ChapterID == id && r.Accepted {
			out.AIRecords++
		}
	}
	return out, nil
}
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

type SearchResult struct {
	Title     string `json:"title"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	URL       string `json:"url"`
	ChapterID string `json:"chapterId"`
	Score     int    `json:"score"`
}

func Search(p Project, query string) []SearchResult {
	out := []SearchResult{}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return out
	}
	terms := strings.Fields(query)
	if len(terms) == 1 && utf8.RuneCountInString(query) > 3 {
		rs := []rune(query)
		for i := 0; i < len(rs)-1; i++ {
			terms = append(terms, string(rs[i:i+2]))
		}
	}
	add := func(title, body, source, url, id string) {
		for i, para := range strings.Split(body, "\n") {
			score := 0
			low := strings.ToLower(para)
			if strings.Contains(low, query) {
				score += 20
			}
			for _, t := range terms {
				if strings.Contains(low, t) {
					score++
				}
			}
			if score > 0 {
				out = append(out, SearchResult{title, truncate(para, 450), fmt.Sprintf("%s · 第 %d 段", source, i+1), url, id, score})
			}
		}
	}
	for _, c := range p.Chapters {
		add(c.Title, c.Text, "本项目正文", "", c.ID)
	}
	for _, s := range p.Sources {
		add(s.Title, s.Text, "导入素材", s.URL, "")
	}
	return out
}
func JSONCopy(p Project) Project {
	b, _ := json.Marshal(p)
	var out Project
	json.Unmarshal(b, &out)
	return out
}
