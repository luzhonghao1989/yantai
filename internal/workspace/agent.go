package workspace

import (
	"encoding/json"
	"errors"
	"net/http"
)

var ProfileFields = []struct{ Key, Label string }{
	{"gender", "性别"}, {"birthplace", "出生地"}, {"birthday", "出生日期"},
	{"ethnicity", "种族 / 民族"}, {"appearance", "外貌特征"}, {"family", "家庭背景"},
	{"identity", "身份 / 职业"}, {"affiliation", "所属势力"}, {"relationships", "人物关系"},
}

// ApplyAgent performs partial, ID-based upserts against a single project revision.
// Collection record fields omitted by an agent remain unchanged. Nested arrays
// (versions, beats, bindings) are explicitly replaced rather than guessed.
func ApplyAgent(p Project, input map[string]json.RawMessage) (Project, error) {
	var expected int64
	if err := json.Unmarshal(input["revision"], &expected); err != nil {
		return p, errors.New("需要整数 revision")
	}
	if expected != p.Revision {
		return p, ErrConflict
	}
	b, _ := json.Marshal(p)
	var result map[string]json.RawMessage
	json.Unmarshal(b, &result)
	fields := map[string]bool{"name": true, "genre": true, "description": true, "outline": true, "rules": true, "style": true, "banned": true}
	collections := map[string]bool{"volumes": true, "stages": true, "chapters": true, "entities": true, "storylines": true, "sources": true}
	for key, value := range input {
		if key == "revision" {
			continue
		}
		if fields[key] {
			var v string
			if json.Unmarshal(value, &v) != nil {
				return p, errors.New(key + " 必须是字符串")
			}
			result[key] = value
			continue
		}
		if !collections[key] {
			return p, errors.New("不支持的写入字段：" + key)
		}
		var updates []map[string]json.RawMessage
		if string(value) == "null" || json.Unmarshal(value, &updates) != nil {
			return p, errors.New(key + " 必须是记录数组")
		}
		var records []map[string]json.RawMessage
		json.Unmarshal(result[key], &records)
		seen := map[string]bool{}
		for _, patch := range updates {
			var id string
			if raw, ok := patch["id"]; ok && json.Unmarshal(raw, &id) != nil {
				return p, errors.New("id 必须是字符串")
			}
			if id == "" {
				id = ID()
				patch["id"], _ = json.Marshal(id)
			}
			if seen[id] {
				return p, errors.New("同一集合不能重复写入 ID")
			}
			seen[id] = true
			index := -1
			for i, r := range records {
				var existing string
				json.Unmarshal(r["id"], &existing)
				if existing == id {
					index = i
					break
				}
			}
			if index < 0 {
				record := map[string]json.RawMessage{}
				// Initialize optional array fields so older and new records work in the UI.
				for _, f := range []string{"entityIds", "excludedIds", "versions", "beats"} {
					record[f] = json.RawMessage("[]")
				}
				if key == "chapters" {
					record["content"] = EmptyDoc("")
					record["text"] = json.RawMessage(`""`)
				}
				records = append(records, record)
				index = len(records) - 1
			}
			for k, v := range patch {
				records[index][k] = v
			}
			if key == "chapters" {
				if _, hasContent := patch["content"]; hasContent {
					return p, errors.New("Agent 接口仅接受 text 正文，富文本请使用完整项目 PUT")
				}
				if raw, ok := patch["text"]; ok {
					var text string
					if json.Unmarshal(raw, &text) != nil {
						return p, errors.New("text 必须是字符串")
					}
					records[index]["content"] = EmptyDoc(text)
				}
			}
		}
		result[key], _ = json.Marshal(records)
	}
	b, _ = json.Marshal(result)
	var next Project
	if err := json.Unmarshal(b, &next); err != nil {
		return p, err
	}
	if err := Validate(next); err != nil {
		return p, err
	}
	return next, nil
}

func (s *Server) registerAgent(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/projects/{id}/agent/apply", func(w http.ResponseWriter, r *http.Request) {
		var input map[string]json.RawMessage
		if err := decode(w, r, &input); err != nil {
			fail(w, err)
			return
		}
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		p, err = ApplyAgent(p, input)
		if err != nil {
			fail(w, err)
			return
		}
		p, err = s.Store.Save(p)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, p)
	})
}
