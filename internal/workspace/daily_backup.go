package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type BackupSettings struct {
	Enabled       bool `json:"enabled"`
	RetentionDays int  `json:"retentionDays"`
}
type BackupProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type DailyBackup struct {
	Kind      string          `json:"kind,omitempty"`
	ID        string          `json:"id"`
	CreatedAt string          `json:"createdAt"`
	Projects  []BackupProject `json:"projects"`
	Errors    []string        `json:"errors,omitempty"`
}

func (s *Store) BackupSettings() (BackupSettings, error) {
	v := BackupSettings{true, 30}
	err := readSyncJSON(filepath.Join(s.Root, "backup-settings.json"), &v)
	if err == nil && (v.RetentionDays < 1 || v.RetentionDays > 36500) {
		err = errors.New("备份保留天数配置无效，未执行清理")
	}
	return v, err
}
func (s *Store) SaveBackupSettings(v BackupSettings) error {
	if v.RetentionDays < 1 || v.RetentionDays > 36500 {
		return errors.New("备份保留天数须为 1 至 36500")
	}
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	return writeSyncJSON(filepath.Join(s.Root, "backup-settings.json"), v)
}
func (s *Store) ListDailyBackups() ([]DailyBackup, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "daily-backups"))
	if os.IsNotExist(err) {
		return []DailyBackup{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []DailyBackup{}
	for _, entry := range entries {
		if !entry.IsDir() || !validID.MatchString(entry.Name()) {
			continue
		}
		var b DailyBackup
		if err = readSyncJSON(filepath.Join(s.Root, "daily-backups", entry.Name(), "manifest.json"), &b); err != nil {
			return nil, err
		}
		if b.ID == entry.Name() {
			result = append(result, b)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt > result[j].CreatedAt })
	return result, nil
}
func (s *Store) RunDailyBackup(force bool) (DailyBackup, error) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	cfg, err := s.BackupSettings()
	if err != nil {
		return DailyBackup{}, err
	}
	if !force && !cfg.Enabled {
		return DailyBackup{}, nil
	}
	now := time.Now()
	previous, err := s.ListDailyBackups()
	if err != nil {
		return DailyBackup{}, err
	}
	if !force {
		for _, b := range previous {
			created, e := time.Parse(time.RFC3339Nano, b.CreatedAt)
			if e == nil && created.In(now.Location()).Format("2006-01-02") == now.Format("2006-01-02") && len(b.Errors) == 0 && b.Kind != "before-delete" {
				return b, nil
			}
		}
	}
	b := DailyBackup{Kind: "daily", ID: ID(), CreatedAt: now.UTC().Format(time.RFC3339Nano), Projects: []BackupProject{}}
	dir := filepath.Join(s.Root, "daily-backups", b.ID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return b, err
	}
	// Saved project JSON and embedded images are portable. Never copy a live SQLite file.
	s.mu.Lock()
	list, e := s.List()
	if e != nil {
		s.mu.Unlock()
		return b, e
	}
	for _, entry := range list {
		p, e := s.Load(entry.ID)
		if e == nil {
			e = s.hydrateImages(&p)
		}
		if e == nil {
			e = writeSyncJSON(filepath.Join(dir, entry.ID+".json"), p)
		}
		if e != nil {
			b.Errors = append(b.Errors, entry.Name+": "+e.Error())
			continue
		}
		b.Projects = append(b.Projects, BackupProject{entry.ID, entry.Name})
	}
	deletions, e := s.projectDeletions()
	if e == nil {
		e = writeSyncJSON(filepath.Join(dir, "deleted-projects.json"), deletions)
	}
	if e != nil {
		b.Errors = append(b.Errors, "删除记录备份失败："+e.Error())
	}
	s.mu.Unlock()
	if err = writeSyncJSON(filepath.Join(dir, "manifest.json"), b); err != nil {
		return b, err
	}
	// Prune only after a fully successful new backup. Invalid/incomplete folders are retained.
	if len(b.Errors) == 0 {
		for _, old := range previous {
			created, e := time.Parse(time.RFC3339Nano, old.CreatedAt)
			if len(old.Errors) > 0 || e != nil || !created.Before(now.AddDate(0, 0, -cfg.RetentionDays)) || !validID.MatchString(old.ID) {
				continue
			}
			backupRoot, e := filepath.Abs(filepath.Join(s.Root, "daily-backups"))
			if e != nil {
				return b, e
			}
			resolved, e := filepath.EvalSymlinks(backupRoot)
			if e != nil {
				return b, e
			}
			if resolved != backupRoot {
				return b, errors.New("备份目录不能包含符号链接")
			}
			target := filepath.Join(backupRoot, old.ID)
			if filepath.Dir(target) != backupRoot {
				return b, errors.New("备份清理路径越界")
			}
			info, e := os.Lstat(target)
			if e != nil || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if e = os.RemoveAll(target); e != nil {
				return b, e
			}
		}
	}
	if len(b.Errors) > 0 {
		return b, fmt.Errorf("部分项目备份失败：%v", b.Errors)
	}
	return b, nil
}
func (s *Store) RestoreDailyBackup(backupID, projectID string) (Project, error) {
	if !validID.MatchString(backupID) || !validID.MatchString(projectID) {
		return Project{}, errors.New("备份标识无效")
	}
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	var p Project
	path := filepath.Join(s.Root, "daily-backups", backupID, projectID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	if p.ID != projectID {
		return p, errors.New("备份项目不匹配")
	}
	if err = Validate(p); err != nil {
		return p, err
	}
	// Restore as a new project; never overwrite active work or revive a deleted ID.
	p.ID = ID()
	p.Name += " · 备份恢复"
	p.Revision = 1
	returnProject, err := s.applySync(p, "")
	return returnProject.Project, err
}
func (s *Store) StartDailyBackups(ctx context.Context) {
	go func() {
		tick := time.NewTicker(time.Hour)
		defer tick.Stop()
		for {
			if _, err := s.RunDailyBackup(false); err != nil {
				log.Printf("Yantai daily backup: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}
func (s *Server) registerBackups(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/backups", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := s.Store.BackupSettings()
		if err != nil {
			fail(w, err)
			return
		}
		list, err := s.Store.ListDailyBackups()
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]any{"settings": cfg, "backups": list, "remote": s.Remote})
	})
	mux.HandleFunc("PUT /api/backups/settings", func(w http.ResponseWriter, r *http.Request) {
		var cfg BackupSettings
		if err := decode(w, r, &cfg); err != nil {
			fail(w, err)
			return
		}
		if err := s.Store.SaveBackupSettings(cfg); err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, cfg)
	})
	mux.HandleFunc("POST /api/backups", func(w http.ResponseWriter, r *http.Request) {
		b, err := s.Store.RunDailyBackup(true)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, b)
	})
	mux.HandleFunc("POST /api/backups/{backup}/projects/{project}/restore", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Store.RestoreDailyBackup(r.PathValue("backup"), r.PathValue("project"))
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, p)
	})
}
