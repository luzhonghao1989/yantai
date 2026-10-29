package workspace

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyBackupRestoreRetentionAndFailure(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	cfg, err := s.BackupSettings()
	if err != nil || !cfg.Enabled || cfg.RetentionDays != 30 {
		t.Fatal(cfg, err)
	}
	p, _ := s.Create("备份原稿", true)
	p.Chapters[0].Text = "保存的旧正文"
	p.Chapters[0].Content = EmptyDoc(p.Chapters[0].Text)
	p.Images = []ProjectImage{{ID: ID(), Name: "地图.svg", Kind: "map", Data: "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="5" height="5"/></svg>`))}}
	p.AIRuns = []AIRun{{ID: ID(), ChapterID: p.Chapters[0].ID, Mode: "review", Text: "保留审稿", CreatedAt: Now()}}
	p, err = s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.RunDailyBackup(false)
	if err != nil || len(first.Projects) != 1 {
		t.Fatal(first, err)
	}
	again, err := s.RunDailyBackup(false)
	if err != nil || first.ID != again.ID {
		t.Fatal("daily backup repeated", err)
	}
	syncEdit(t, s, p.ID, "当前新稿")
	restored, err := s.RestoreDailyBackup(first.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID == p.ID || restored.Chapters[0].Text != "保存的旧正文" || len(restored.AIRuns) != 1 || len(restored.Images) != 1 {
		t.Fatal("restore lost data", restored)
	}
	if err = s.hydrateImages(&restored); err != nil || restored.Images[0].Data == "" {
		t.Fatal("restore image missing", err)
	}
	current, _ := s.Load(p.ID)
	if current.Chapters[0].Text != "当前新稿" {
		t.Fatal("restore overwrote current project")
	}
	first.CreatedAt = time.Now().AddDate(0, 0, -31).UTC().Format(time.RFC3339Nano)
	oldDir := filepath.Join(s.Root, "daily-backups", first.ID)
	if err = writeSyncJSON(filepath.Join(oldDir, "manifest.json"), first); err != nil {
		t.Fatal(err)
	}
	badID := ID()
	badDir := filepath.Join(s.Root, "projects", badID)
	os.MkdirAll(badDir, 0700)
	os.WriteFile(filepath.Join(badDir, "novel.db"), []byte("not sqlite"), 0600)
	partial, err := s.RunDailyBackup(true)
	if err == nil || len(partial.Errors) == 0 {
		t.Fatal("missing backup failure", partial, err)
	}
	if _, err = os.Stat(oldDir); err != nil {
		t.Fatal("old backup removed on failure")
	}
	// Move a corrupt fixture aside; never discard the only copy when testing pruning.
	os.Rename(badDir, filepath.Join(s.Root, "corrupt-fixture"))
	if _, err = s.RunDailyBackup(true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatal("expired successful backup retained", err)
	}
	if err = s.SaveBackupSettings(BackupSettings{false, 30}); err != nil {
		t.Fatal(err)
	}
	b, err := s.RunDailyBackup(false)
	if err != nil || b.ID != "" {
		t.Fatal("disabled auto backup ran", err)
	}
	if _, err = s.RunDailyBackup(true); err != nil {
		t.Fatal("manual backup disabled", err)
	}
	if _, err = s.RestoreDailyBackup("../outside", p.ID); err == nil {
		t.Fatal("path traversal allowed")
	}
}

func TestServerDeletionStopsOldClientsAndPreservesOfflineWork(t *testing.T) {
	remote, _ := NewStore(t.TempDir())
	server := httptest.NewTLSServer((&Server{Store: remote, Remote: true, AuthKey: syncTestKey}).SyncHandler())
	defer server.Close()
	a, b, c := syncTestClient(t, server), syncTestClient(t, server), syncTestClient(t, server)
	p, _ := remote.Create("删除测试", true)
	runSyncTest(t, a, "downloaded")
	runSyncTest(t, b, "downloaded")
	snap, _ := remote.syncSnapshot(p.ID)
	// C has the old project but no baseline (e.g. an older installation).
	if _, err := c.applySync(snap.Project, ""); err != nil {
		t.Fatal(err)
	}
	changed := syncEdit(t, b, p.ID, "离线修改必须保留")
	if _, err := remote.DeleteServerProject(p.ID, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatal("stale delete allowed", err)
	}
	d, err := remote.DeleteServerProject(p.ID, snap.Hash)
	if err != nil {
		t.Fatal(err)
	}
	runSyncTest(t, a, "deleted")
	if _, err = a.Load(p.ID); err == nil {
		t.Fatal("unchanged copy still active")
	}
	runSyncTest(t, b, "remote-deleted")
	runSyncTest(t, c, "remote-deleted")
	retained, _ := b.Load(p.ID)
	if retained.Chapters[0].Text != changed.Chapters[0].Text {
		t.Fatal("offline edits lost")
	}
	restarted, _ := NewStore(remote.Root)
	if _, err = restarted.applySync(snap.Project, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("old client resurrected deleted ID", err)
	}
	if _, err = restarted.Save(snap.Project); !errors.Is(err, ErrConflict) {
		t.Fatal("stale server tab revived deleted project", err)
	}
	manifest, _ := remote.syncManifest()
	if len(manifest.Projects) != 0 || len(manifest.Deleted) != 1 || manifest.Deleted[0].ID != d.ID {
		t.Fatal("deletion missing from manifest", manifest)
	}
	h, _ := syncHash(retained)
	if _, err = b.ResolveDeletedProject(p.ID, "stale", true); !errors.Is(err, ErrConflict) {
		t.Fatal("stale resolution discarded edits", err)
	}
	independent, err := b.ResolveDeletedProject(p.ID, h, true)
	if err != nil || independent.ID == p.ID || independent.Chapters[0].Text != changed.Chapters[0].Text {
		t.Fatal("independent copy failed", err)
	}
	report, err := b.RunSync(context.Background(), nil)
	if err != nil || len(report.Items) != 1 || report.Items[0].ID != independent.ID || report.Items[0].Action != "uploaded" {
		t.Fatal(report, err)
	}
	backups, _ := remote.ListDailyBackups()
	if len(backups) != 1 || backups[0].Kind != "before-delete" {
		t.Fatal("delete checkpoint missing")
	}
	restored, err := remote.RestoreDailyBackup(backups[0].ID, p.ID)
	if err != nil || restored.ID == p.ID {
		t.Fatal("deleted checkpoint cannot restore", err)
	}
}

func TestSelectedProjectsLimitUploadsDownloadsAndDeletes(t *testing.T) {
	remote, _ := NewStore(t.TempDir())
	server := httptest.NewTLSServer((&Server{Store: remote, Remote: true, AuthKey: syncTestKey}).SyncHandler())
	defer server.Close()
	a, _ := remote.Create("选中", true)
	b, _ := remote.Create("未选", true)
	client := syncTestClient(t, server)
	local, _ := client.Create("本机不上传", true)
	cfg, _ := client.SyncSettings()
	cfg.ProjectScope = "selected"
	cfg.SelectedProjects = []string{a.ID}
	client.SaveSyncSettings(cfg)
	item := runSyncTest(t, client, "downloaded")
	if item.ID != a.ID {
		t.Fatal("wrong project downloaded")
	}
	if _, err := client.Load(b.ID); err == nil {
		t.Fatal("unselected project downloaded")
	}
	if _, err := remote.Load(local.ID); err == nil {
		t.Fatal("unselected local uploaded")
	}
	snap, _ := remote.syncSnapshot(a.ID)
	if _, err := remote.DeleteServerProject(a.ID, snap.Hash); err != nil {
		t.Fatal(err)
	}
	cfg.SelectedProjects = []string{}
	client.SaveSyncSettings(cfg)
	report, err := client.RunSync(context.Background(), nil)
	if err != nil || len(report.Items) != 0 {
		t.Fatal(report, err)
	}
	if _, err = client.Load(a.ID); err != nil {
		t.Fatal("unselected deletion processed")
	}
	cfg.SelectedProjects = []string{a.ID}
	client.SaveSyncSettings(cfg)
	runSyncTest(t, client, "deleted")
}

func TestBackupHTTPAndServerDeleteRoutes(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	p, _ := s.Create("HTTP", true)
	h := (&Server{Store: s, Remote: true, AuthKey: syncTestKey}).Handler()
	// Existing authenticated Handler must also protect new backup APIs.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/backups", nil))
	if w.Code == 200 {
		t.Fatal("backups exposed without authentication")
	}
	b, err := s.RunDailyBackup(true)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(s.Root, "daily-backups", b.ID, p.ID+".json"))
	var stored Project
	if err != nil || json.Unmarshal(body, &stored) != nil || stored.ID != p.ID {
		t.Fatal("invalid portable backup", err)
	}
}
