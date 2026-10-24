package workspace

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeleteCorruptLocalAndRedownloadKeepsServerAndBackup(t *testing.T) {
	remote, _ := NewStore(t.TempDir())
	server := httptest.NewTLSServer((&Server{Store: remote, Remote: true, AuthKey: syncTestKey}).SyncHandler())
	defer server.Close()
	local := syncTestClient(t, server)
	p, err := remote.Create("服务端正式稿", true)
	if err != nil {
		t.Fatal(err)
	}
	runSyncTest(t, local, "downloaded")
	before, err := remote.syncSnapshot(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(local.Root, "projects", p.ID)
	raw, err := os.ReadFile(filepath.Join(dir, "novel.db"))
	if err != nil {
		t.Fatal(err)
	}
	raw[100] = 0xff
	files := map[string][]byte{"novel.db": raw, "novel.db-wal": []byte("preserve WAL evidence"), "novel.db-shm": []byte("preserve SHM evidence")}
	for name, data := range files {
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var draft Project
	encoded, _ := json.Marshal(p)
	if err = json.Unmarshal(encoded, &draft); err != nil {
		t.Fatal(err)
	}
	draft.Chapters[0].Text = "客户端未保存的独有草稿"
	out, err := local.DeleteLocalProject(p.ID, &draft)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("local directory still exists", err)
	}
	for name, data := range files {
		got, err := os.ReadFile(filepath.Join(out.BackupPath, name))
		if err != nil || !bytes.Equal(got, data) {
			t.Fatal("backup changed", name, err)
		}
	}
	drafts, err := filepath.Glob(filepath.Join(out.BackupPath, "unsaved-draft-*.json"))
	if err != nil || len(drafts) != 1 {
		t.Fatal("draft not archived", err)
	}
	body, _ := os.ReadFile(drafts[0])
	if !strings.Contains(string(body), "客户端未保存的独有草稿") {
		t.Fatal("draft missing")
	}
	runSyncTest(t, local, "downloaded") // Old baseline must not turn a local removal into a conflict.
	after, err := remote.syncSnapshot(p.ID)
	if err != nil || before.Hash != after.Hash {
		t.Fatal("server original changed", err)
	}
	restored, err := local.Load(p.ID)
	if err != nil || restored.Chapters[0].Text != p.Chapters[0].Text {
		t.Fatal("server copy not restored", err)
	}
	if _, err = os.Stat(out.BackupPath); err != nil {
		t.Fatal("backup lost after resync", err)
	}
}

func TestLocalDeleteWaitsForOpenDatabase(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	p, err := s.Create("正在读取的项目", false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.db(p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() { close(started); _, err := s.DeleteLocalProject(p.ID, nil); finished <- err }()
	<-started
	select {
	case err := <-finished:
		t.Fatal("directory moved before reader closed", err)
	case <-time.After(100 * time.Millisecond):
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM state").Scan(&count); err != nil || count != 1 {
		t.Fatal("reader disrupted", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deletion did not resume after closing reader")
	}
}

func TestDeleteLocalRollbackAndOtherBaselines(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	p, _ := s.Create("待移除", false)
	other, _ := s.Create("另一本", false)
	state := syncState{Peers: map[string]map[string]string{"a": {p.ID: "old", other.ID: "keep"}, "b": {p.ID: "old2", other.ID: "keep2"}}}
	statePath := filepath.Join(s.Root, "sync-state.json")
	if err := writeSyncJSON(statePath, state); err != nil {
		t.Fatal(err)
	}
	// Failed state persistence must put the original project back.
	if err := os.Mkdir(statePath+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteLocalProject(p.ID, nil); err == nil {
		t.Fatal("state failure ignored")
	}
	if _, err := s.Load(p.ID); err != nil {
		t.Fatal("rollback lost project", err)
	}
	if err := os.Remove(statePath + ".tmp"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteLocalProject(p.ID, nil); err != nil {
		t.Fatal(err)
	}
	state = syncState{}
	if err := readSyncJSON(statePath, &state); err != nil {
		t.Fatal(err)
	}
	for peer, hashes := range state.Peers {
		if _, ok := hashes[p.ID]; ok {
			t.Fatal("baseline retained", peer)
		}
		if hashes[other.ID] == "" {
			t.Fatal("other baseline cleared")
		}
	}
	if _, err := s.Load(other.ID); err != nil {
		t.Fatal("other project changed", err)
	}
	if _, err := s.DeleteLocalProject("../outside", nil); err == nil {
		t.Fatal("invalid ID accepted")
	}
	s.syncRunMu.Lock()
	_, err := s.DeleteLocalProject(other.ID, nil)
	s.syncRunMu.Unlock()
	if err == nil {
		t.Fatal("deleted during synchronization")
	}
}

func TestLocalDeleteHTTPAndRemoteDeny(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	p, _ := s.Create("API移除", false)
	h := (&Server{Store: s, Token: "desktop-token"}).Handler()
	r := httptest.NewRequest("OPTIONS", "http://127.0.0.1/api/projects/"+p.ID+"/local", nil)
	r.Header.Set("Origin", "http://wails.localhost")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "DELETE") {
		t.Fatal("desktop DELETE preflight denied", w.Code)
	}
	r = httptest.NewRequest("DELETE", "http://127.0.0.1/api/projects/"+p.ID+"/local", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Yantai-Token", "desktop-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out LocalDeleteResult
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.BackupPath == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	remote, _ := NewStore(t.TempDir())
	rp, _ := remote.Create("服务端保留", false)
	h = (&Server{Store: remote, Remote: true, AuthKey: syncTestKey}).Handler()
	r = httptest.NewRequest("DELETE", "http://server.example/api/projects/"+rp.ID+"/local", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+syncTestKey)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("server deletion permitted", w.Code)
	}
	if _, err := remote.Load(rp.ID); err != nil {
		t.Fatal("server original deleted", err)
	}
}
