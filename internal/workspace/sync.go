package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SyncSettings struct {
	ProjectScope     string   `json:"projectScope"`
	SelectedProjects []string `json:"selectedProjects"`
	ServerURL        string   `json:"serverUrl"`
	Token            string   `json:"token"`
	TLSFingerprint   string   `json:"tlsFingerprint"`
	HasToken         bool     `json:"hasToken"`
	Automatic        bool     `json:"automatic"`
	IntervalMinutes  int      `json:"intervalMinutes"`
}
type SyncEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Hash string `json:"hash"`
}
type SyncManifest struct {
	Deleted  []ProjectDeletion `json:"deleted,omitempty"`
	ServerID string            `json:"serverId"`
	Projects []SyncEntry       `json:"projects"`
	Issues   []SyncIssue       `json:"issues,omitempty"`
}
type SyncIssue struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}
type SyncSnapshot struct {
	Hash    string  `json:"hash"`
	Project Project `json:"project"`
}
type SyncItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Action     string `json:"action"`
	Message    string `json:"message"`
	LocalHash  string `json:"localHash,omitempty"`
	RemoteHash string `json:"remoteHash,omitempty"`
}
type SyncReport struct {
	StartedAt  string     `json:"startedAt"`
	FinishedAt string     `json:"finishedAt"`
	ServerURL  string     `json:"serverUrl"`
	Items      []SyncItem `json:"items"`
}
type SyncResolution struct {
	ID         string `json:"id"`
	Direction  string `json:"direction"`
	LocalHash  string `json:"localHash"`
	RemoteHash string `json:"remoteHash"`
}
type syncState struct {
	Peers map[string]map[string]string `json:"peers"`
	Last  *SyncReport                  `json:"last,omitempty"`
}

func readSyncJSON(path string, v any) error {
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func writeSyncJSON(path string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if e = os.WriteFile(path+".tmp", b, 0600); e != nil {
		return e
	}
	return os.Rename(path+".tmp", path)
}
func (s *Store) SyncSettings() (SyncSettings, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	v := SyncSettings{IntervalMinutes: 5}
	e := readSyncJSON(filepath.Join(s.Root, "sync-settings.json"), &v)
	return v, e
}
func validateSyncSettings(v *SyncSettings) error {
	if v.ProjectScope != "" && v.ProjectScope != "all" && v.ProjectScope != "selected" {
		return errors.New("未知项目同步范围")
	}
	for _, id := range v.SelectedProjects {
		if !validID.MatchString(id) {
			return errors.New("同步项目ID无效")
		}
	}

	u, e := url.Parse(strings.TrimSpace(v.ServerURL))
	if e != nil || u.Host == "" || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("同步必须使用 HTTPS 加密地址，不含路径、用户名、查询参数或片段")
	}
	v.ServerURL = strings.TrimRight(u.String(), "/")
	v.TLSFingerprint = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(v.TLSFingerprint), ":", ""))
	if v.TLSFingerprint != "" {
		b, err := hex.DecodeString(v.TLSFingerprint)
		if err != nil || len(b) != sha256.Size {
			return errors.New("证书指纹须为 SHA-256 的64位十六进制字符")
		}
	}
	return nil
}
func (s *Store) SaveSyncSettings(v SyncSettings) (SyncSettings, error) {
	if e := validateSyncSettings(&v); e != nil {
		return v, e
	}
	if v.IntervalMinutes < 1 || v.IntervalMinutes > 10080 {
		return v, errors.New("定时同步间隔须为 1 至 10080 分钟")
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if v.Token == "" && v.HasToken {
		var old SyncSettings
		if e := readSyncJSON(filepath.Join(s.Root, "sync-settings.json"), &old); e != nil {
			return v, e
		}
		v.Token = old.Token
	}
	if len(v.Token) < 16 {
		return v, errors.New("请填写服务端访问密钥，至少16字符")
	}
	v.HasToken = false
	e := writeSyncJSON(filepath.Join(s.Root, "sync-settings.json"), v)
	return v, e
}
func publicSyncSettings(v SyncSettings) SyncSettings {
	v.HasToken = v.Token != ""
	v.Token = ""
	return v
}
func syncHash(p Project) (string, error) {
	p.Revision = 0
	p.UpdatedAt = ""
	p.Images = append([]ProjectImage(nil), p.Images...)
	for i := range p.Images {
		im := &p.Images[i]
		if im.Data != "" {
			b, m, e := decodeProjectImage(im.Data)
			if e != nil {
				return "", e
			}
			h := sha256.Sum256(b)
			im.SHA = hex.EncodeToString(h[:])
			im.MIME = m
			im.Size = int64(len(b))
		}
		im.Data = ""
	}
	b, e := json.Marshal(p)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func (s *Store) syncSnapshot(id string) (SyncSnapshot, error) {
	p, e := s.Load(id)
	if e != nil {
		return SyncSnapshot{}, e
	}
	h, e := syncHash(p)
	if e != nil {
		return SyncSnapshot{}, e
	}
	if e = s.hydrateImages(&p); e != nil {
		return SyncSnapshot{}, e
	}
	return SyncSnapshot{h, p}, nil
}
func (s *Store) syncManifest() (SyncManifest, error) {
	s.syncMu.Lock()
	idPath := filepath.Join(s.Root, "sync-server-id")
	b, e := os.ReadFile(idPath)
	if os.IsNotExist(e) {
		b = []byte(ID())
		e = os.WriteFile(idPath, b, 0600)
	}
	s.syncMu.Unlock()
	if e != nil {
		return SyncManifest{}, e
	}
	out := SyncManifest{ServerID: string(b), Projects: []SyncEntry{}}
	out.Deleted, e = s.projectDeletions()
	if e != nil {
		return out, e
	}
	if !validID.MatchString(out.ServerID) {
		return out, errors.New("服务端同步标识损坏，请检查数据目录")
	}
	list, e := s.List()
	if e != nil {
		return out, e
	}
	for _, entry := range list {
		if entry.Error != "" {
			out.Issues = append(out.Issues, SyncIssue{entry.ID, entry.Error})
			continue
		}
		p, e := s.Load(entry.ID)
		if e != nil {
			out.Issues = append(out.Issues, SyncIssue{entry.ID, e.Error()})
			continue
		}
		h, e := syncHash(p)
		if e != nil {
			out.Issues = append(out.Issues, SyncIssue{entry.ID, e.Error()})
			continue
		}
		out.Projects = append(out.Projects, SyncEntry{p.ID, p.Name, h})
	}
	return out, nil
}

// Compare and write inside the same transaction/lock used by ordinary saves.
// Revision numbers remain local to each device; snapshots/history are retained.
func (s *Store) applySync(p Project, expected string) (SyncSnapshot, error) {
	if e := Validate(p); e != nil {
		return SyncSnapshot{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applySyncLocked(p, expected)
}

// Caller holds mu, including when resolving a deleted project's independent copy.
func (s *Store) applySyncLocked(p Project, expected string) (SyncSnapshot, error) {
	deletion, deleteErr := s.projectDeletion(p.ID)
	if deleteErr != nil {
		return SyncSnapshot{}, deleteErr
	}
	if deletion.ID != "" {
		return SyncSnapshot{}, fmt.Errorf("项目已在服务端删除，旧副本不能重新上传：%w", ErrConflict)
	}
	if expected != "" {
		if _, e := os.Stat(filepath.Join(s.Root, "projects", p.ID, "novel.db")); e != nil {
			return SyncSnapshot{}, ErrConflict
		}
	}
	db, e := s.db(p.ID, true)
	if e != nil {
		return SyncSnapshot{}, e
	}
	defer db.Close()
	tx, e := db.Begin()
	if e != nil {
		return SyncSnapshot{}, e
	}
	defer tx.Rollback()
	var oldBody string
	var revision int64
	e = tx.QueryRow(`SELECT revision,body FROM state WHERE id=1`).Scan(&revision, &oldBody)
	exists := e == nil
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return SyncSnapshot{}, e
	}
	if exists {
		var old Project
		if e = json.Unmarshal([]byte(oldBody), &old); e != nil {
			return SyncSnapshot{}, e
		}
		h, e := syncHash(old)
		if e != nil {
			return SyncSnapshot{}, e
		}
		if expected == "" || h != expected {
			return SyncSnapshot{}, ErrConflict
		}
	} else if expected != "" {
		return SyncSnapshot{}, ErrConflict
	}
	if e = s.prepareImages(&p); e != nil {
		return SyncSnapshot{}, e
	}
	if exists {
		if _, e = tx.Exec(`INSERT INTO history VALUES(?,?,?,?)`, revision, Now(), "同步前版本", oldBody); e != nil {
			return SyncSnapshot{}, e
		}
	}
	p.Revision = revision + 1
	p.UpdatedAt = Now()
	b, e := json.Marshal(p)
	if e != nil {
		return SyncSnapshot{}, e
	}
	if _, e = tx.Exec(`INSERT INTO state(id,revision,body) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,body=excluded.body`, p.Revision, string(b)); e != nil {
		return SyncSnapshot{}, e
	}
	if _, e = tx.Exec(`DELETE FROM history WHERE revision NOT IN (SELECT revision FROM history ORDER BY revision DESC LIMIT 200)`); e != nil {
		return SyncSnapshot{}, e
	}
	if e = tx.Commit(); e != nil {
		return SyncSnapshot{}, e
	}
	h, e := syncHash(p)
	return SyncSnapshot{h, p}, e
}

func syncRemote(ctx context.Context, cfg SyncSettings, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, e := json.Marshal(in)
		if e != nil {
			return e
		}
		body = strings.NewReader(string(b))
	}
	req, e := http.NewRequestWithContext(ctx, method, cfg.ServerURL+"/api/sync"+path, body)
	if e != nil {
		return errors.New("服务端地址无效")
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client, e := syncHTTPClient(cfg)
	if e != nil {
		return e
	}
	defer client.CloseIdleConnections()
	res, e := client.Do(req)
	if e != nil {
		return errors.New("无法连接同步服务端，请检查地址、网络和证书")
	}
	defer res.Body.Close()
	if res.StatusCode == 409 {
		return ErrConflict
	}
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return errors.New("同步服务端访问密钥无效")
	}
	if res.StatusCode != 200 {
		var detail struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&detail) == nil && detail.Error != "" {
			return fmt.Errorf("服务端同步接口失败（HTTP %d）：%s", res.StatusCode, detail.Error)
		}
		return fmt.Errorf("同步服务端返回 HTTP %d", res.StatusCode)
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(out); e != nil {
		return errors.New("同步服务端响应无效或过大")
	}
	return nil
}

func (s *Store) RunSync(ctx context.Context, resolutions []SyncResolution) (SyncReport, error) {
	if !s.syncRunMu.TryLock() {
		return SyncReport{}, errors.New("同步正在进行，请等待完成")
	}
	defer s.syncRunMu.Unlock()
	cfg, e := s.SyncSettings()
	if e != nil {
		return SyncReport{}, e
	}
	if cfg.ServerURL == "" || cfg.Token == "" {
		return SyncReport{}, errors.New("请先保存服务端地址和访问密钥")
	}
	if e = validateSyncSettings(&cfg); e != nil {
		return SyncReport{}, e
	}
	report := SyncReport{StartedAt: Now(), ServerURL: cfg.ServerURL, Items: []SyncItem{}}
	var remote SyncManifest
	if e = syncRemote(ctx, cfg, "GET", "/manifest", nil, &remote); e != nil {
		return report, e
	}
	if !validID.MatchString(remote.ServerID) {
		return report, errors.New("服务端同步标识无效")
	}
	s.syncMu.Lock()
	state := syncState{}
	e = readSyncJSON(filepath.Join(s.Root, "sync-state.json"), &state)
	s.syncMu.Unlock()
	if e != nil {
		return report, e
	}
	if state.Peers == nil {
		state.Peers = map[string]map[string]string{}
	}
	baseline := state.Peers[remote.ServerID]
	if baseline == nil {
		baseline = map[string]string{}
	}
	deleted := map[string]ProjectDeletion{}
	for _, d := range remote.Deleted {
		if !validID.MatchString(d.ID) {
			return report, errors.New("删除记录无效")
		}
		deleted[d.ID] = d
	}
	selected := map[string]bool{}
	for _, id := range cfg.SelectedProjects {
		selected[id] = true
	}
	remoteMap := map[string]SyncEntry{}
	ids := map[string]bool{}
	for _, p := range remote.Projects {
		if !validID.MatchString(p.ID) || !imageSHA.MatchString(p.Hash) {
			return report, errors.New("服务端项目清单无效")
		}
		remoteMap[p.ID] = p
		ids[p.ID] = true
	}
	local, e := s.List()
	if e != nil {
		return report, fmt.Errorf("本机项目目录读取失败：%w", e)
	}
	blocked := map[string]string{}
	for _, issue := range remote.Issues {
		if !validID.MatchString(issue.ID) {
			return report, errors.New("服务端异常项目标识无效")
		}
		blocked[issue.ID] = "服务端项目无法读取，已跳过且未覆盖：" + issue.Message
		ids[issue.ID] = true
	}
	localIDs := map[string]bool{}
	for _, p := range local {
		localIDs[p.ID] = true
		ids[p.ID] = true
		if p.Error != "" {
			blocked[p.ID] = "本机项目无法读取，已跳过且未覆盖：" + p.Error
		}
	}
	choices := map[string]SyncResolution{}
	for _, r := range resolutions {
		if r.Direction != "upload" && r.Direction != "download" {
			return report, errors.New("未知冲突处理方式")
		}
		choices[r.ID] = r
	}
	ordered := []string{}
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, id := range ordered {
		if cfg.ProjectScope == "selected" && !selected[id] {
			continue
		}
		if e = ctx.Err(); e != nil {
			return report, e
		}
		r, hasRemote := remoteMap[id]
		hasLocal := localIDs[id]
		var l Project
		lh := ""
		name := r.Name
		item := SyncItem{ID: id, Name: name}
		if item.Name == "" {
			item.Name = "项目 " + id
		}
		if message := blocked[id]; message != "" {
			item.Action = "error"
			item.Message = message
			report.Items = append(report.Items, item)
			continue
		}
		if hasLocal {
			l, e = s.Load(id)
			if e == nil {
				lh, e = syncHash(l)
			}
			if e != nil {
				item.Action = "error"
				item.Message = "本机项目读取失败：" + e.Error()
				report.Items = append(report.Items, item)
				continue
			}
			name = l.Name
		}
		item.Name = name
		item.LocalHash = lh
		item.RemoteHash = r.Hash
		if d, ok := deleted[id]; ok {
			item.Name = d.Name
			if !hasLocal {
				continue
			}
			if baseline[id] != "" && lh == baseline[id] {
				err := s.archiveUnchangedDeleted(id, lh)
				if err != nil {
					item.Action = "error"
					item.Message = "服务端已删除；本机备份未完成，保留本机项目：" + err.Error()
				} else {
					item.Action = "deleted"
					item.Message = "服务端已删除，本机未修改副本已移入备份目录"
				}
			} else {
				item.Action = "remote-deleted"
				item.Message = "服务端已删除此项目；本机有修改或无同步基线，已阻止上传。可保留为独立副本或确认移除。"
			}
			report.Items = append(report.Items, item)
			continue
		}
		action := "conflict"
		base := baseline[id]
		switch {
		case hasLocal && hasRemote && lh == r.Hash:
			action = "unchanged"
		case hasLocal && !hasRemote && base == "":
			action = "upload"
		case !hasLocal && hasRemote && base == "":
			action = "download"
		case hasLocal && hasRemote && base != "" && lh == base:
			action = "download"
		case hasLocal && hasRemote && base != "" && r.Hash == base:
			action = "upload"
		}
		if choice, ok := choices[id]; ok && action != "unchanged" {
			if choice.LocalHash != lh || choice.RemoteHash != r.Hash {
				item.Action = "conflict"
				item.Message = "选择后内容又发生变化，请重新核对双方版本"
				report.Items = append(report.Items, item)
				continue
			}
			if (choice.Direction == "upload" && !hasLocal) || (choice.Direction == "download" && !hasRemote) {
				item.Action = "conflict"
				item.Message = "所选一端已不存在，请先保留现有项目"
				report.Items = append(report.Items, item)
				continue
			}
			action = choice.Direction
		}
		switch action {
		case "unchanged":
			baseline[id] = lh
			item.Action = "unchanged"
			item.Message = "两端一致"
		case "conflict":
			item.Action = "conflict"
			item.Message = "两端均有变化或首次连接内容不同，请选择保留哪一端；未覆盖任何内容"
		case "upload":
			snap, err := s.syncSnapshot(id)
			if err == nil && snap.Hash != lh {
				err = ErrConflict
			}
			var received SyncSnapshot
			if err == nil {
				err = syncRemote(ctx, cfg, "PUT", "/projects/"+id, map[string]any{"expectedHash": r.Hash, "project": snap.Project}, &received)
			}
			if err != nil {
				item.Action = "error"
				if errors.Is(err, ErrConflict) {
					item.Action = "conflict"
				}
				item.Message = err.Error()
			} else if received.Hash != snap.Hash {
				item.Action = "error"
				item.Message = "上传后的内容校验不一致，请重新同步"
			} else {
				baseline[id] = received.Hash
				item.Action = "uploaded"
				item.Message = "已上传到服务端"
			}
		case "download":
			var snap SyncSnapshot
			err := syncRemote(ctx, cfg, "GET", "/projects/"+id, nil, &snap)
			if err == nil && (snap.Project.ID != id || snap.Hash != r.Hash) {
				err = ErrConflict
			}
			if err == nil {
				var hash string
				hash, err = syncHash(snap.Project)
				if err == nil && hash != snap.Hash {
					err = errors.New("下载内容校验不一致")
				}
			}
			if err == nil {
				_, err = s.applySync(snap.Project, lh)
				if err != nil && !errors.Is(err, ErrConflict) {
					err = fmt.Errorf("本机项目 %s 写入失败：%w", id, err)
				}
			}
			if err != nil {
				item.Action = "error"
				if errors.Is(err, ErrConflict) {
					item.Action = "conflict"
				}
				item.Message = err.Error()
			} else {
				baseline[id] = snap.Hash
				item.Action = "downloaded"
				item.Message = "已下载到本机"
			}
		}
		report.Items = append(report.Items, item)
	}
	report.FinishedAt = Now()
	state.Peers[remote.ServerID] = baseline
	state.Last = &report
	s.syncMu.Lock()
	e = writeSyncJSON(filepath.Join(s.Root, "sync-state.json"), state)
	s.syncMu.Unlock()
	return report, e
}

func (s *Server) registerSync(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sync/catalog", func(w http.ResponseWriter, r *http.Request) {
		local, err := s.Store.List()
		if err != nil {
			fail(w, err)
			return
		}
		items := map[string]SyncEntry{}
		for _, p := range local {
			items[p.ID] = SyncEntry{ID: p.ID, Name: p.Name}
		}
		warning := ""
		if !s.Remote {
			cfg, e := s.Store.SyncSettings()
			if e != nil {
				fail(w, e)
				return
			}
			if cfg.ServerURL != "" {
				var remote SyncManifest
				e = syncRemote(r.Context(), cfg, "GET", "/manifest", nil, &remote)
				if e != nil {
					warning = e.Error()
				} else {
					for _, p := range remote.Projects {
						items[p.ID] = p
					}
				}
			}
		}
		out := []SyncEntry{}
		for _, p := range items {
			out = append(out, p)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		reply(w, 200, map[string]any{"projects": out, "warning": warning})
	})
	mux.HandleFunc("GET /api/sync/settings", func(w http.ResponseWriter, r *http.Request) {
		if s.Remote {
			reply(w, 200, map[string]any{"remote": true})
			return
		}
		cfg, e := s.Store.SyncSettings()
		if e != nil {
			fail(w, e)
			return
		}
		s.Store.syncMu.Lock()
		state := syncState{}
		e = readSyncJSON(filepath.Join(s.Store.Root, "sync-state.json"), &state)
		s.Store.syncMu.Unlock()
		if e != nil {
			fail(w, e)
			return
		}
		reply(w, 200, map[string]any{"remote": false, "settings": publicSyncSettings(cfg), "last": state.Last})
	})
	mux.HandleFunc("PUT /api/sync/settings", func(w http.ResponseWriter, r *http.Request) {
		if s.Remote {
			fail(w, errors.New("请在客户端配置同步"))
			return
		}
		var in SyncSettings
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		cfg, e := s.Store.SaveSyncSettings(in)
		if e != nil {
			fail(w, e)
			return
		}
		reply(w, 200, publicSyncSettings(cfg))
	})
	mux.HandleFunc("POST /api/sync/run", func(w http.ResponseWriter, r *http.Request) {
		if s.Remote {
			fail(w, errors.New("请从客户端发起同步"))
			return
		}
		var in struct {
			Resolutions []SyncResolution `json:"resolutions"`
		}
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		out, e := s.Store.RunSync(r.Context(), in.Resolutions)
		if e != nil {
			fail(w, e)
			return
		}
		reply(w, 200, out)
	})
	s.registerSyncRemote(mux)
}

func (s *Server) registerSyncRemote(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sync/manifest", func(w http.ResponseWriter, r *http.Request) {
		if !s.Remote {
			fail(w, errors.New("目标未启用服务端模式"))
			return
		}
		out, e := s.Store.syncManifest()
		if e != nil {
			fail(w, e)
			return
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("GET /api/sync/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !s.Remote {
			fail(w, errors.New("目标未启用服务端模式"))
			return
		}
		out, e := s.Store.syncSnapshot(r.PathValue("id"))
		if e != nil {
			fail(w, e)
			return
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("PUT /api/sync/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !s.Remote {
			fail(w, errors.New("目标未启用服务端模式"))
			return
		}
		var in struct {
			ExpectedHash string  `json:"expectedHash"`
			Project      Project `json:"project"`
		}
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		if in.Project.ID != r.PathValue("id") {
			fail(w, errors.New("项目ID不匹配"))
			return
		}
		out, e := s.Store.applySync(in.Project, in.ExpectedHash)
		if e != nil {
			fail(w, e)
			return
		}
		reply(w, 200, out)
	})
}
