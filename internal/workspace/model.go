package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Volume struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Outline   string   `json:"outline"`
	EntityIDs []string `json:"entityIds"`
}
type Stage struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	VolumeID  string   `json:"volumeId"`
	Outline   string   `json:"outline"`
	EntityIDs []string `json:"entityIds"`
}
type Chapter struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	VolumeID    string          `json:"volumeId"`
	StageID     string          `json:"stageId"`
	Outline     string          `json:"outline"`
	Summary     string          `json:"summary"`
	Content     json.RawMessage `json:"content"`
	Text        string          `json:"text"`
	EntityIDs   []string        `json:"entityIds"`
	ExcludedIDs []string        `json:"excludedIds"`
}
type Version struct {
	StageID   string `json:"stageId"`
	Traits    string `json:"traits"`
	Likes     string `json:"likes"`
	State     string `json:"state"`
	Knowledge string `json:"knowledge"`
	Note      string `json:"note"`
}
type Entity struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Name     string            `json:"name"`
	Base     string            `json:"base"`
	Profile  map[string]string `json:"profile,omitempty"`
	Global   bool              `json:"global"`
	Versions []Version         `json:"versions"`
}
type Beat struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	ChapterID string `json:"chapterId"`
	Status    string `json:"status"`
	Detail    string `json:"detail"`
}
type Storyline struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Beats []Beat `json:"beats"`
}
type Source struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	URL   string `json:"url"`
}
type AIRun struct {
	InputText string `json:"inputText,omitempty"`
	ID        string `json:"id"`
	ChapterID string `json:"chapterId"`
	Mode      string `json:"mode"`
	Model     string `json:"model"`
	Text      string `json:"text"`
	Accepted  bool   `json:"accepted"`
	CreatedAt string `json:"createdAt"`
}
type Project struct {
	Images      []ProjectImage `json:"images,omitempty"`
	Schema      int            `json:"schema"`
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Genre       string         `json:"genre"`
	Description string         `json:"description"`
	Outline     string         `json:"outline"`
	Rules       string         `json:"rules"`
	Style       string         `json:"style"`
	Banned      string         `json:"banned"`
	Revision    int64          `json:"revision"`
	UpdatedAt   string         `json:"updatedAt"`
	Volumes     []Volume       `json:"volumes"`
	Stages      []Stage        `json:"stages"`
	Chapters    []Chapter      `json:"chapters"`
	Entities    []Entity       `json:"entities"`
	Storylines  []Storyline    `json:"storylines"`
	Sources     []Source       `json:"sources"`
	AIRuns      []AIRun        `json:"aiRuns"`
}

func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Now() string { return time.Now().UTC().Format(time.RFC3339) }
func EmptyDoc(text string) json.RawMessage {
	paragraphs := []any{}
	for _, p := range strings.Split(text, "\n") {
		children := []any{}
		if p != "" {
			children = append(children, map[string]any{"type": "text", "text": p})
		}
		paragraphs = append(paragraphs, map[string]any{"type": "paragraph", "content": children})
	}
	b, _ := json.Marshal(map[string]any{"type": "doc", "content": paragraphs})
	return b
}
func NewProject(name string, demo bool) Project {
	v, s, c := ID(), ID(), ID()
	p := Project{Schema: 1, ID: ID(), Name: name, Genre: "长篇小说", Revision: 1, UpdatedAt: Now(), Volumes: []Volume{{ID: v, Name: "第一卷", EntityIDs: []string{}}}, Stages: []Stage{{ID: s, Name: "初始阶段", VolumeID: v, EntityIDs: []string{}}}, Chapters: []Chapter{{ID: c, Title: "第一章", VolumeID: v, StageID: s, Content: EmptyDoc(""), EntityIDs: []string{}, ExcludedIDs: []string{}}}, Entities: []Entity{}, Storylines: []Storyline{}, Sources: []Source{}, AIRuns: []AIRun{}}
	if demo {
		p.Name = "山海来信"
		p.Genre = "东方幻想"
		p.Description = "一封来自山海之外的信，让小镇少女踏上寻找故乡真相的旅程。"
		p.Outline = "林晚收到失踪父亲的来信，穿越雾山寻找旧日渡口。主线从信件之谜推进至山海边界的真相。"
		p.Rules = "灵力需要借助器物引导。林晚尚未学会御剑，不能在初始阶段独立飞行。"
		p.Style = "克制、清透，以具体动作和环境细节传递情绪。对白自然，避免过多解释。"
		p.Banned = "命运的齿轮\n不禁陷入了沉思"
		p.Volumes[0].Name = "第一卷 · 雾山来客"
		p.Volumes[0].Outline = "收到来信，离开小镇，在渡口遇见守信人。"
		p.Stages[0].Name = "小镇启程"
		p.Stages[0].Outline = "林晚确认信件来自父亲，决定在天亮前出发。不得提前揭露父亲身份。"
		s2 := ID()
		p.Stages = append(p.Stages, Stage{ID: s2, Name: "渡口相逢", VolumeID: v, Outline: "抵达旧渡口，学会用铜铃辨认山雾。", EntityIDs: []string{}})
		text := "雨在傍晚停了。\n林晚推开木窗时，屋檐上的水珠正一滴一滴落进石缸。巷子里没有人，只有一只灰猫蹲在墙头，盯着她手中的信。\n信封很薄，边角被水浸得发软。她没有急着拆，只用指腹摸了摸封口处那道熟悉的折痕。\n父亲每次寄信，都要把右上角往里折一点。\n可他已经失踪三年了。\n林晚坐回桌前，将灯芯拨亮。纸上只有一行字：\n“天亮之前，到旧渡口来。”\n她看了很久，直到灯花轻轻爆了一声。"
		p.Chapters[0].Title = "第一章 · 雨停之后"
		p.Chapters[0].Text = text
		p.Chapters[0].Content = EmptyDoc(text)
		p.Chapters[0].Outline = "用信封上的折痕确认父亲的线索，以旧渡口之约作为章末悬念。"
		e1, e2, e3 := ID(), ID(), ID()
		p.Entities = []Entity{{ID: e1, Kind: "character", Name: "林晚", Base: "十七岁，住在青石镇，父亲三年前失踪。", Global: true, Versions: []Version{{StageID: s, Traits: "谨慎，遇到与父亲有关的事情会冒险。", Likes: "雨后的木头气味，热茶。", State: "尚未修炼，独自住在旧宅。", Knowledge: "只知道父亲曾常去旧渡口。"}, {StageID: s2, Traits: "仍有戒心，开始主动追问。", Likes: "热茶。", State: "抵达渡口，学会铜铃辨雾。", Knowledge: "知道铜铃可指示雾中道路。"}}}, {ID: e2, Kind: "item", Name: "父亲的来信", Base: "薄纸信封，右上角向内折。", Versions: []Version{{StageID: s, State: "林晚持有，封口已受潮。", Note: "只写着天亮前到旧渡口。"}}}, {ID: e3, Kind: "location", Name: "青石镇旧宅", Base: "临着一条窄巷，窗外有石缸。", Versions: []Version{{StageID: s, State: "傍晚雨停，屋内只有林晚。"}}}}
		p.Stages[0].EntityIDs = []string{e1, e2, e3}
		p.Stages[1].EntityIDs = []string{e1, e2}
		p.Storylines = []Storyline{{ID: ID(), Name: "来信之谜", Color: "#28584b", Beats: []Beat{{ID: ID(), Title: "折痕与来信", ChapterID: c, Status: "planted", Detail: "信封折痕指向父亲，但真实寄信人尚未揭晓。"}, {ID: ID(), Title: "渡口守信人", Status: "planned", Detail: "在后续章节引入线索，不能在第一章提前揭晓。"}}}}
	}
	return p
}
func Validate(p Project) error {
	if err := validateImages(p.Images); err != nil {
		return err
	}
	if len(p.Volumes) == 0 || len(p.Stages) == 0 {
		return errors.New("项目至少需要一个卷和一个阶段")
	}
	if p.Schema != 1 {
		return errors.New("不支持的项目格式版本")
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("小说名称不能为空")
	}
	ids := map[string]bool{}
	add := func(id string) error {
		if id == "" || ids[id] {
			return errors.New("记录 ID 缺失或重复")
		}
		ids[id] = true
		return nil
	}
	volumes, stages, entities, chapters := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, v := range p.Volumes {
		if err := add(v.ID); err != nil {
			return err
		}
		volumes[v.ID] = true
	}
	for _, s := range p.Stages {
		if err := add(s.ID); err != nil {
			return err
		}
		stages[s.ID] = true
		if !volumes[s.VolumeID] {
			return errors.New("阶段引用不存在的卷")
		}
	}
	for _, e := range p.Entities {
		if err := add(e.ID); err != nil {
			return err
		}
		entities[e.ID] = true
		if e.Kind != "character" && e.Kind != "item" && e.Kind != "location" {
			return errors.New("未知设定类型")
		}
		seen := map[string]bool{}
		for _, v := range e.Versions {
			if !stages[v.StageID] || seen[v.StageID] {
				return errors.New("设定版本的阶段缺失或重复")
			}
			seen[v.StageID] = true
		}
	}
	binding := func(list []string) error {
		for _, id := range list {
			if !entities[id] {
				return errors.New("资料绑定引用不存在的设定")
			}
		}
		return nil
	}
	for _, v := range p.Volumes {
		if err := binding(v.EntityIDs); err != nil {
			return err
		}
	}
	for _, s := range p.Stages {
		if err := binding(s.EntityIDs); err != nil {
			return err
		}
	}
	for _, c := range p.Chapters {
		if err := add(c.ID); err != nil {
			return err
		}
		chapters[c.ID] = true
		if !volumes[c.VolumeID] || !stages[c.StageID] {
			return errors.New("章节引用不存在的卷或阶段")
		}
		if err := binding(c.EntityIDs); err != nil {
			return err
		}
		if err := binding(c.ExcludedIDs); err != nil {
			return err
		}
		if !json.Valid(c.Content) {
			return errors.New("正文格式无效")
		}
	}
	for _, l := range p.Storylines {
		if err := add(l.ID); err != nil {
			return err
		}
		for _, b := range l.Beats {
			if err := add(b.ID); err != nil {
				return err
			}
			if b.ChapterID != "" && !chapters[b.ChapterID] {
				return errors.New("故事线引用不存在的章节")
			}
		}
	}
	for _, s := range p.Sources {
		if err := add(s.ID); err != nil {
			return err
		}
	}
	return nil
}

type ResolvedEntity struct {
	Entity    Entity   `json:"entity"`
	Version   *Version `json:"version"`
	Inherited bool     `json:"inherited"`
}

func Resolve(p Project, c Chapter) []ResolvedEntity {
	order := map[string]int{}
	current := -1
	for i, s := range p.Stages {
		order[s.ID] = i
		if s.ID == c.StageID {
			current = i
		}
	}
	selected := map[string]bool{}
	for _, e := range p.Entities {
		if e.Global {
			selected[e.ID] = true
		}
	}
	for _, v := range p.Volumes {
		if v.ID == c.VolumeID {
			for _, id := range v.EntityIDs {
				selected[id] = true
			}
		}
	}
	for _, s := range p.Stages {
		if s.ID == c.StageID {
			for _, id := range s.EntityIDs {
				selected[id] = true
			}
		}
	}
	for _, id := range c.EntityIDs {
		selected[id] = true
	}
	for _, id := range c.ExcludedIDs {
		delete(selected, id)
	}
	result := []ResolvedEntity{}
	for _, e := range p.Entities {
		if !selected[e.ID] {
			continue
		}
		r := ResolvedEntity{Entity: e}
		best := -1
		for _, v := range e.Versions {
			i, ok := order[v.StageID]
			if ok && i <= current && i > best {
				copy := v
				r.Version = &copy
				best = i
				r.Inherited = v.StageID != c.StageID
			}
		}
		result = append(result, r)
	}
	return result
}
func FindChapter(p Project, id string) (Chapter, error) {
	for _, c := range p.Chapters {
		if c.ID == id {
			return c, nil
		}
	}
	return Chapter{}, fmt.Errorf("章节不存在")
}
