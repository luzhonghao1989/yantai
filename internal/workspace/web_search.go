package workspace

import (
	"context"
	"encoding/xml"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Bing's public RSS search provides a keyless fallback; configured keys use Brave.
func searchRSS(ctx context.Context, endpoint string) ([]SearchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Yantai/0.5.2")
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, errors.New("免密钥联网搜索连接失败，请稍后重试或在设置中配置 Brave Search")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("免密钥搜索暂不可用，请稍后重试或在设置中配置 Brave Search")
	}
	var feed struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Items []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Description string `xml:"description"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&feed); err != nil {
		return nil, errors.New("搜索服务未返回有效摘要，请稍后重试或配置 Brave Search")
	}
	out := []SearchResult{}
	for _, item := range feed.Channel.Items {
		u, err := url.Parse(item.Link)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			continue
		}
		out = append(out, SearchResult{Title: html.UnescapeString(strings.TrimSpace(item.Title)), URL: item.Link, Text: html.UnescapeString(item.Description), Source: "Bing · 网页摘要"})
		if len(out) == 8 {
			break
		}
	}
	return out, nil
}
