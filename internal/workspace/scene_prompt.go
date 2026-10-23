package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type scenePromptRequest struct {
	ChapterID   string `json:"chapterId"`
	Revision    int64  `json:"revision"`
	Instruction string `json:"instruction"`
	EntityID    string `json:"entityId"`
}
type scenePromptResult struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negativePrompt"`
}

func parseScenePrompt(text string) (scenePromptResult, error) {
	raw := strings.TrimSpace(text)
	if strings.HasPrefix(raw, "```") {
		if start := strings.Index(raw, "\n"); start >= 0 && strings.HasSuffix(raw, "```") {
			raw = strings.TrimSpace(strings.TrimSuffix(raw[start+1:], "```"))
		}
	}
	var out scenePromptResult
	if json.Unmarshal([]byte(raw), &out) != nil || strings.TrimSpace(out.Prompt) == "" || len(out.Prompt) > 60000 || len(out.NegativePrompt) > 30000 || len(out.Title) > 500 || len(out.Description) > 12000 {
		return out, errors.New("AI 未生成有效的场景提示词，请重新生成；尚未提交 ComfyUI")
	}
	if strings.TrimSpace(out.Title) == "" {
		out.Title = "本章场景图"
	}
	return out, nil
}
func generateChapterScenePrompt(ctx context.Context, cfg AIConfig, p Project, in scenePromptRequest) (scenePromptResult, error) {
	if p.Revision != in.Revision {
		return scenePromptResult{}, ErrConflict
	}
	chapter, err := FindChapter(p, in.ChapterID)
	if err != nil {
		return scenePromptResult{}, err
	}
	if strings.TrimSpace(chapter.Text) == "" {
		return scenePromptResult{}, errors.New("请先填写本章正文，AI 会根据本章实际内容生成场景图")
	}
	if len(in.Instruction) > 12000 {
		return scenePromptResult{}, errors.New("画面补充要求过长，请缩短后重试")
	}
	instruction := "根据当前章节完整正文选择一个代表性场景，生成正向和负向画面提示词。"
	if in.EntityID != "" {
		found := false
		for _, e := range Resolve(p, chapter) {
			if e.Entity.ID == in.EntityID && e.Entity.Kind == "location" {
				instruction += "\n作者指定优先绘制场景：" + e.Entity.Name + "。仍须依据本章实际发生的内容。"
				found = true
				break
			}
		}
		if !found {
			return scenePromptResult{}, errors.New("指定场景不在本章当前有效资料中，请重新选择")
		}
	}
	if strings.TrimSpace(in.Instruction) != "" {
		instruction += "\n作者补充的画面要求：" + in.Instruction
	}
	cp, err := BuildContext(p, AIRequest{ChapterID: in.ChapterID, Revision: in.Revision, Mode: "scene_prompt", Instruction: instruction}, cfg)
	if err != nil {
		return scenePromptResult{}, err
	}
	// Put the visual task after the full chapter and lore to keep the output goal clear.
	ordered := make([]ContextSection, 0, len(cp.Sections))
	for _, section := range cp.Sections {
		if section.Name != "任务" {
			ordered = append(ordered, section)
		}
	}
	for _, section := range cp.Sections {
		if section.Name == "任务" {
			ordered = append(ordered, section)
		}
	}
	cp.Sections = ordered
	text, err := Generate(ctx, cfg, cp, "scene_prompt", func(string) error { return nil })
	if err != nil {
		return scenePromptResult{}, err
	}
	return parseScenePrompt(text)
}
func (s *Server) registerScenePrompt(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/projects/{id}/scene-prompt", func(w http.ResponseWriter, r *http.Request) {
		var in scenePromptRequest
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		cfg, err := s.Store.Config()
		if err != nil {
			fail(w, err)
			return
		}
		out, err := generateChapterScenePrompt(r.Context(), cfg, p, in)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, out)
	})
}
