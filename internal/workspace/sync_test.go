package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const syncTestKey = "fixture-server-secret-not-production"

func syncTestClient(t *testing.T, server *httptest.Server) *Store {
	t.Helper()
	s, e := NewStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(server.Certificate().Raw)
	_, e = s.SaveSyncSettings(SyncSettings{ServerURL: server.URL, Token: syncTestKey, TLSFingerprint: hex.EncodeToString(digest[:]), IntervalMinutes: 5})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func runSyncTest(t *testing.T, s *Store, action string, resolutions ...SyncResolution) SyncItem {
	t.Helper()
	report, e := s.RunSync(context.Background(), resolutions)
	if e != nil {
		t.Fatal(e)
	}
	if len(report.Items) != 1 || report.Items[0].Action != action {
		t.Fatalf("expected %s: %#v", action, report)
	}
	return report.Items[0]
}
func syncEdit(t *testing.T, s *Store, id, text string) Project {
	t.Helper()
	p, e := s.Load(id)
	if e != nil {
		t.Fatal(e)
	}
	p.Chapters[0].Text = text
	p.Chapters[0].Content = EmptyDoc(text)
	p, e = s.Save(p)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestSyncTwoClientsAndServer(t *testing.T) {
	remote, _ := NewStore(t.TempDir())
	server := httptest.NewTLSServer((&Server{Store: remote, Remote: true, AuthKey: syncTestKey}).SyncHandler())
	defer server.Close()
	a := syncTestClient(t, server)
	b := syncTestClient(t, server)
	p, _ := a.Create("同步测试", true)
	p = syncEdit(t, a, p.ID, "电脑A第一稿")
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="5" height="5"/></svg>`
	p.Images = []ProjectImage{{ID: ID(), Name: "地图.svg", Kind: "map", Data: "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))}}
	p.AIRuns = []AIRun{{ID: ID(), ChapterID: p.Chapters[0].ID, Mode: "review", Text: "审稿记录", CreatedAt: Now()}}
	p, e := a.Save(p)
	if e != nil {
		t.Fatal(e)
	}
	runSyncTest(t, a, "uploaded")
	runSyncTest(t, b, "downloaded")
	runSyncTest(t, a, "unchanged")
	copied, _ := b.Load(p.ID)
	if copied.Chapters[0].Text != "电脑A第一稿" || len(copied.AIRuns) != 1 || len(copied.Images) != 1 {
		t.Fatal("incomplete project sync")
	}
	if _, e = os.Stat(filepath.Join(b.Root, "images", copied.Images[0].SHA)); e != nil {
		t.Fatal(e)
	}
	syncEdit(t, remote, p.ID, "服务端网页修改")
	runSyncTest(t, a, "downloaded")
	runSyncTest(t, b, "downloaded")
	syncEdit(t, a, p.ID, "电脑A第二稿")
	runSyncTest(t, a, "uploaded")
	runSyncTest(t, b, "downloaded")
	syncEdit(t, a, p.ID, "A离线修改")
	syncEdit(t, b, p.ID, "B离线修改")
	runSyncTest(t, a, "uploaded")
	conflict := runSyncTest(t, b, "conflict")
	local, _ := b.Load(p.ID)
	remoteP, _ := remote.Load(p.ID)
	if local.Chapters[0].Text != "B离线修改" || remoteP.Chapters[0].Text != "A离线修改" {
		t.Fatal("conflict overwrote content")
	}
	syncEdit(t, remote, p.ID, "冲突显示后服务端又修改")
	runSyncTest(t, b, "conflict", SyncResolution{p.ID, "upload", conflict.LocalHash, conflict.RemoteHash})
	conflict = runSyncTest(t, b, "conflict")
	runSyncTest(t, b, "downloaded", SyncResolution{p.ID, "download", conflict.LocalHash, conflict.RemoteHash})
	history, e := b.History(p.ID)
	if e != nil || len(history) < 2 {
		t.Fatal("sync history missing", e)
	}
	syncEdit(t, b, p.ID, "再次本机改")
	syncEdit(t, remote, p.ID, "再次网页改")
	conflict = runSyncTest(t, b, "conflict")
	runSyncTest(t, b, "uploaded", SyncResolution{p.ID, "upload", conflict.LocalHash, conflict.RemoteHash})
	restarted, _ := NewStore(b.Root)
	runSyncTest(t, restarted, "unchanged")
	cfg, _ := b.SyncSettings()
	if publicSyncSettings(cfg).Token != "" || !publicSyncSettings(cfg).HasToken {
		t.Fatal("credential exposed")
	}
	cfg.Token = ""
	cfg.HasToken = true
	if _, e = b.SaveSyncSettings(cfg); e != nil {
		t.Fatal(e)
	}
	manifest, _ := remote.syncManifest()
	again, _ := remote.syncManifest()
	if manifest.ServerID != again.ServerID {
		t.Fatal("unstable server identity")
	}
}
func TestSyncFirstConflictAndCAS(t *testing.T) {
	remote, _ := NewStore(t.TempDir())
	server := httptest.NewTLSServer((&Server{Store: remote, Remote: true, AuthKey: syncTestKey}).SyncHandler())
	defer server.Close()
	a := syncTestClient(t, server)
	p, _ := a.Create("初次冲突", true)
	p = syncEdit(t, a, p.ID, "本机版本")
	var copy Project
	// Clone through JSON so nested slices do not modify the local fixture.
	raw, _ := json.Marshal(p)
	_ = json.Unmarshal(raw, &copy)
	copy.Chapters[0].Text = "服务端版本"
	copy.Chapters[0].Content = EmptyDoc("服务端版本")
	if _, e := remote.applySync(copy, ""); e != nil {
		t.Fatal(e)
	}
	item := runSyncTest(t, a, "conflict")
	if _, e := remote.applySync(p, ""); !errors.Is(e, ErrConflict) {
		t.Fatal("existing project replaced without expected hash")
	}
	runSyncTest(t, a, "uploaded", SyncResolution{p.ID, "upload", item.LocalHash, item.RemoteHash})
	old, _ := a.syncSnapshot(p.ID)
	syncEdit(t, a, p.ID, "同步期间继续写作")
	if _, e := a.applySync(old.Project, old.Hash); !errors.Is(e, ErrConflict) {
		t.Fatal("concurrent local edit overwritten")
	}
	cfg, _ := a.SyncSettings()
	cfg.Token = "wrong-key-at-least-16"
	_, _ = a.SaveSyncSettings(cfg)
	if _, e := a.RunSync(context.Background(), nil); e == nil {
		t.Fatal("bad credentials accepted")
	}
}
func TestServerAuthenticationAndOrigin(t *testing.T) {
	store, _ := NewStore(t.TempDir())
	handler := (&Server{Store: store, Remote: true, AuthKey: syncTestKey}).Handler()
	request := func(method, path, body, auth, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://server.example"+path, strings.NewReader(body))
		if method != "GET" {
			r.Header.Set("Content-Type", "application/json")
		}
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/projects", "", "", "", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("GET", "/api/server-info", "", "", "", nil); w.Code != 200 || strings.Contains(w.Body.String(), syncTestKey) {
		t.Fatal(w.Body.String())
	}
	w := request("POST", "/api/auth/login", `{"token":"`+syncTestKey+`"}`, "", "http://server.example", nil)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal("login failed", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || strings.Contains(cookie.Value, syncTestKey) {
		t.Fatal("unsafe session cookie")
	}
	if w = request("GET", "/api/projects", "", "", "", cookie); w.Code != 200 {
		t.Fatal("browser session denied")
	}
	if w = request("POST", "/api/auth/login", `{"token":"`+syncTestKey+`"}`, "", "https://unrelated.example", nil); w.Code != 403 {
		t.Fatal("cross-origin login accepted")
	}
	if w = request("GET", "/api/sync/manifest", "", syncTestKey, "", nil); w.Code != 200 {
		t.Fatal("authenticated sync denied")
	}
	if w = request("POST", "/api/sync/run", `{}`, syncTestKey, "", nil); w.Code != 400 {
		t.Fatal("server initiated client sync")
	}
	if w = request("POST", "/api/auth/logout", `{}`, "", "http://server.example", cookie); w.Code != 200 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout failed")
	}
	local := (&Server{Store: store}).Handler()
	r := httptest.NewRequest("GET", "http://server.example/api/projects", nil)
	w = httptest.NewRecorder()
	local.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("local mode exposed to network host")
	}
}
