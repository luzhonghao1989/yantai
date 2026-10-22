package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Optional detector adapter contract, not a built-in classifier or authorship verdict.
type Detection struct {
	Score       *float64 `json:"score"`
	Label       string   `json:"label"`
	Model       string   `json:"model"`
	Explanation string   `json:"explanation"`
	Language    string   `json:"language"`
	Revision    int64    `json:"revision"`
	ChapterID   string   `json:"chapterId"`
	Words       int      `json:"words"`
}

func Detect(ctx context.Context, cfg AIConfig, p Project, id string) (Detection, error) {
	c, err := FindChapter(p, id)
	if err != nil {
		return Detection{}, err
	}
	if cfg.DetectorURL == "" {
		return Detection{}, errors.New("请先在模型与设置中配置独立的 AI 检测服务。内置文风规则不能判断作者身份。")
	}
	if WordCount(c.Text) < 200 {
		return Detection{}, errors.New("当前正文不足 200 字，首版不对短文本请求来源检测")
	}
	body, _ := json.Marshal(map[string]string{"text": c.Text, "language": "zh"})
	req, err := makeRequest(ctx, "POST", cfg.DetectorURL, cfg.DetectorKey, body)
	if err != nil {
		return Detection{}, err
	}
	client := &http.Client{Timeout: 45 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return Detection{}, errors.New("检测服务连接失败")
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return Detection{}, fmt.Errorf("检测服务返回 HTTP %d", res.StatusCode)
	}
	var out Detection
	if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return out, errors.New("检测服务响应不符合 JSON 协议")
	}
	if out.Score != nil && (*out.Score < 0 || *out.Score > 1) {
		return out, errors.New("检测服务分数必须在 0 至 1 之间")
	}
	if out.Model == "" || out.Explanation == "" || out.Language == "" {
		return out, errors.New("检测服务须返回 model、explanation、language 字段")
	}
	out.Revision = p.Revision
	out.ChapterID = c.ID
	out.Words = WordCount(c.Text)
	return out, nil
}
