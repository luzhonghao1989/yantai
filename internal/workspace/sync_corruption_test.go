package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncCorruptProjectIsVisibleAndNeverOverwritten(t *testing.T) {
	for _, side := range []string{"client", "server"} {
		t.Run(side, func(t *testing.T) {
			remote, _ := NewStore(t.TempDir())
			server := httptest.NewTLSServer((&Server{Store: remote, Remote: true, AuthKey: syncTestKey}).SyncHandler())
			defer server.Close()
			local := syncTestClient(t, server)
			p, err := local.Create("保留两端原稿", true)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = remote.applySync(p, ""); err != nil {
				t.Fatal(err)
			}
			healthy, err := remote.Create("正常项目仍能下载", false)
			if err != nil {
				t.Fatal(err)
			}
			broken := local
			good := remote
			if side == "server" {
				broken = remote
				good = local
			}
			path := filepath.Join(broken.Root, "projects", p.ID, "novel.db")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Keep the SQLite header but invalidate the first schema b-tree page.
			raw[100] = 0xff
			if err = os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = broken.Load(p.ID); err == nil || !strings.Contains(err.Error(), p.ID) {
				t.Fatal("unidentified corruption", err)
			}
			books, err := broken.List()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, book := range books {
				if book.ID == p.ID {
					found = book.Error != ""
				}
			}
			if !found {
				t.Fatal("bad project vanished from shelf")
			}
			before, err := good.syncSnapshot(p.ID)
			if err != nil {
				t.Fatal(err)
			}
			report, err := local.RunSync(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			var blocked, downloaded bool
			for _, item := range report.Items {
				if item.ID == p.ID {
					blocked = item.Action == "error" && strings.Contains(item.Message, p.ID)
					if side == "client" && !strings.Contains(item.Message, "本机") {
						t.Fatal(item)
					}
				}
				if item.ID == healthy.ID {
					downloaded = item.Action == "downloaded"
				}
			}
			if !blocked || !downloaded {
				t.Fatalf("unrelated sync blocked or bad project overwritten: %#v", report)
			}
			// Even explicit conflict choices cannot use an unreadable side as a target/source.
			for _, direction := range []string{"upload", "download"} {
				if _, err = local.RunSync(context.Background(), []SyncResolution{{ID: p.ID, Direction: direction}}); err != nil {
					t.Fatal(err)
				}
			}
			after, err := good.syncSnapshot(p.ID)
			if err != nil || before.Hash != after.Hash {
				t.Fatal("healthy original changed", err)
			}
			unchanged, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(raw, unchanged) {
				t.Fatal("corrupt evidence changed", err)
			}
		})
	}
}

func TestSyncRemoteErrorIdentifiesServer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply(w, 400, map[string]string{"error": "fixture project failed integrity check"})
	}))
	defer server.Close()
	hash := sha256.Sum256(server.Certificate().Raw)
	cfg := SyncSettings{ServerURL: server.URL, Token: syncTestKey, TLSFingerprint: hex.EncodeToString(hash[:])}
	var out SyncManifest
	err := syncRemote(context.Background(), cfg, "GET", "/manifest", nil, &out)
	if err == nil || !strings.Contains(err.Error(), "服务端") || !strings.Contains(err.Error(), "fixture project") {
		t.Fatal(err)
	}
}

func TestSQLiteEngineIncludesWALResetFix(t *testing.T) {
	store, _ := NewStore(t.TempDir())
	p, err := store.Create("版本验证", false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.db(p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version, check string
	if err = db.QueryRow("SELECT sqlite_version()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "3.51.3" {
		t.Fatal("unexpected SQLite engine", version)
	}
	if err = db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		t.Fatal(check, err)
	}
	// Keep the wire form compatible with clients predating per-project diagnostics.
	var manifest SyncManifest
	if err = json.Unmarshal([]byte(`{"serverId":"legacy","projects":[]}`), &manifest); err != nil || len(manifest.Issues) != 0 {
		t.Fatal(err)
	}
}
