package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ProjectImage struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Note        string `json:"note"`
	SHA         string `json:"sha,omitempty"`
	MIME        string `json:"mime,omitempty"`
	Size        int64  `json:"size"`
	SourceRunID string `json:"sourceRunId,omitempty"`
	CreatedAt   string `json:"createdAt"`
	Data        string `json:"data,omitempty"`
}

var imageSHA = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validateImages(images []ProjectImage) error {
	if len(images) > 200 {
		return errors.New("每个项目最多保存 200 张图片")
	}
	seen := map[string]bool{}
	total := 0
	for _, im := range images {
		if im.ID == "" || seen[im.ID] || strings.TrimSpace(im.Name) == "" || len(im.Name) > 500 || len(im.Note) > 30000 {
			return errors.New("图片 ID、名称或备注无效")
		}
		seen[im.ID] = true
		if im.Kind != "map" && im.Kind != "character" && im.Kind != "scene" && im.Kind != "other" {
			return errors.New("图片分类无效")
		}
		if im.Data == "" && !imageSHA.MatchString(im.SHA) {
			return errors.New("图片内容或引用无效")
		}
		if im.Data == "" && im.MIME != "image/png" && im.MIME != "image/jpeg" && im.MIME != "image/gif" && im.MIME != "image/svg+xml" {
			return errors.New("图片媒体类型无效")
		}
		total += len(im.Data)
		if len(im.Data) > 12<<20 || total > 48<<20 {
			return errors.New("单张图片最多 8 MiB，单次导入图片总量过大")
		}
	}
	return nil
}
func decodeProjectImage(value string) ([]byte, string, error) {
	_, encoded, ok := strings.Cut(value, ",")
	if !ok || !strings.HasPrefix(value, "data:") {
		return nil, "", errors.New("图片须为 base64 图片数据")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) > 8<<20 {
		return nil, "", errors.New("图片编码无效或超过 8 MiB")
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("<")) {
		if err := validateSVG(data); err != nil {
			return nil, "", err
		}
		return data, "image/svg+xml", nil
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 20000 || cfg.Height > 20000 || int64(cfg.Width)*int64(cfg.Height) > 100000000 {
		return nil, "", errors.New("仅支持有效的 PNG、JPEG、GIF、静态 SVG 图片，位图最大 1 亿像素")
	}
	mime := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif"}[format]
	if mime == "" {
		return nil, "", errors.New("不支持的图片格式")
	}
	return data, mime, nil
}
func (s *Store) imagePath(sha string) (string, error) {
	if !imageSHA.MatchString(sha) {
		return "", errors.New("图片引用无效")
	}
	return filepath.Join(s.Root, "images", sha), nil
}
func (s *Store) prepareImages(p *Project) error {
	var total int64
	for i := range p.Images {
		im := &p.Images[i]
		if im.Data != "" {
			data, mime, err := decodeProjectImage(im.Data)
			if err != nil {
				return err
			}
			sha := fmt.Sprintf("%x", sha256.Sum256(data))
			path, _ := s.imagePath(sha)
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			if _, err = os.Stat(path); os.IsNotExist(err) {
				tmp, e := os.CreateTemp(filepath.Dir(path), "image-*")
				if e != nil {
					return e
				}
				_, e = tmp.Write(data)
				closeErr := tmp.Close()
				if e == nil {
					e = closeErr
				}
				if e == nil {
					e = os.Rename(tmp.Name(), path)
				}
				os.Remove(tmp.Name())
				if e != nil {
					return e
				}
			} else if err != nil {
				return err
			}
			im.SHA = sha
			im.MIME = mime
			im.Size = int64(len(data))
			im.Data = ""
		} else {
			path, err := s.imagePath(im.SHA)
			if err != nil {
				return err
			}
			info, err := os.Stat(path)
			if err != nil {
				return errors.New("图片文件缺失，请从完整项目备份重新导入")
			}
			im.Size = info.Size()
		}
		total += im.Size
		if total > 32<<20 {
			return errors.New("每个项目的图片总量最多 32 MiB，以便完整备份与导入")
		}
	}
	return nil
}
func (s *Store) hydrateImages(p *Project) error {
	total := 0
	for i := range p.Images {
		im := &p.Images[i]
		path, err := s.imagePath(im.SHA)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return errors.New("图片文件缺失，无法生成完整备份")
		}
		total += len(data)
		if total > 32<<20 {
			return errors.New("当前完整备份的图片总量上限为 32 MiB，请分项目整理后备份")
		}
		im.Data = "data:" + im.MIME + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	return nil
}
func (s *Server) registerImages(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{id}/images/{image}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Store.Load(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		for _, im := range p.Images {
			if im.ID != r.PathValue("image") {
				continue
			}
			path, err := s.Store.imagePath(im.SHA)
			if err != nil {
				fail(w, err)
				return
			}
			data, err := os.ReadFile(path)
			if err != nil {
				http.Error(w, "图片不存在", 404)
				return
			}
			w.Header().Set("Content-Type", im.MIME)
			w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
			w.Header().Set("Cache-Control", "private, max-age=3600")
			w.Write(data)
			return
		}
		http.Error(w, "图片不存在", 404)
	})
}
func validateSVG(data []byte) error {
	elements := strings.Fields("svg g rect text tspan line circle ellipse polyline polygon path title desc defs marker clipPath")
	allowed := map[string]bool{}
	for _, name := range elements {
		allowed[name] = true
	}
	attrs := map[string]bool{}
	for _, name := range strings.Fields("xmlns viewBox width height role aria-label x y x1 y1 x2 y2 cx cy r rx ry fill stroke stroke-width stroke-dasharray stroke-linecap stroke-linejoin stroke-opacity fill-opacity opacity font-family font-size font-weight text-anchor dominant-baseline paint-order d points transform id markerWidth markerHeight refX refY orient preserveAspectRatio version") {
		attrs[name] = true
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	root := false
	depth := 0
	count := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("SVG 格式无效")
		}
		count++
		if count > 100000 {
			return errors.New("SVG 内容过于复杂")
		}
		switch value := token.(type) {
		case xml.StartElement:
			if !root {
				if value.Name.Local != "svg" {
					return errors.New("图片必须是 SVG 文档")
				}
				root = true
			} else if depth == 0 {
				return errors.New("SVG 不能有多个根元素")
			}
			depth++
			if !allowed[value.Name.Local] || (value.Name.Space != "" && value.Name.Space != "http://www.w3.org/2000/svg") {
				return errors.New("SVG 含不支持的动态或外部元素")
			}
			for _, a := range value.Attr {
				if !attrs[a.Name.Local] || strings.Contains(strings.ToLower(a.Value), "url(") || strings.Contains(a.Value, "\\") {
					return errors.New("SVG 含脚本、样式或外部资源；请导入静态 SVG")
				}
			}
		case xml.EndElement:
			depth--
		case xml.Directive:
			return errors.New("SVG 不允许文档指令")
		case xml.ProcInst:
			if value.Target != "xml" {
				return errors.New("SVG 不允许处理指令")
			}
		}
	}
	if !root || depth != 0 {
		return errors.New("SVG 根元素无效")
	}
	return nil
}
