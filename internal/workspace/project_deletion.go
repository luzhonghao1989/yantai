package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

type ProjectDeletion struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Hash      string `json:"hash"`
	DeletedAt string `json:"deletedAt"`
}

func (s *Store) projectDeletion(id string) (ProjectDeletion, error) {
	if !validID.MatchString(id) {
		return ProjectDeletion{}, errors.New("项目ID无效")
	}
	var d ProjectDeletion
	data, err := os.ReadFile(filepath.Join(s.Root, "deleted-projects", id+".json"))
	if os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	if err = json.Unmarshal(data, &d); err != nil {
		return d, err
	}
	if d.ID != id || !imageSHA.MatchString(d.Hash) || d.DeletedAt == "" {
		return d, errors.New("项目删除记录损坏，已阻止写入")
	}
	return d, nil
}
func (s *Store) projectDeletions() ([]ProjectDeletion, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "deleted-projects"))
	if os.IsNotExist(err) {
		return []ProjectDeletion{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []ProjectDeletion{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) != 37 || filepath.Ext(name) != ".json" {
			continue
		}
		d, e := s.projectDeletion(name[:32])
		if e != nil {
			return nil, e
		}
		if d.ID != "" {
			out = append(out, d)
		}
	}
	return out, nil
}

// expected prevents a delete from discarding work saved since the confirmation dialog.
func (s *Store) DeleteServerProject(id, expected string) (ProjectDeletion, error) {
	if !s.syncRunMu.TryLock() {
		return ProjectDeletion{}, errors.New("同步正在进行，请稍后删除")
	}
	defer s.syncRunMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, err := s.syncSnapshot(id)
	if err != nil {
		return ProjectDeletion{}, err
	}
	if expected == "" || snap.Hash != expected {
		return ProjectDeletion{}, ErrConflict
	}
	if err = s.checkpointDeletedProject(snap); err != nil {
		return ProjectDeletion{}, err
	}
	d := ProjectDeletion{id, snap.Project.Name, snap.Hash, Now()}
	if err = os.MkdirAll(filepath.Join(s.Root, "deleted-projects"), 0700); err != nil {
		return d, err
	}
	if err = writeSyncJSON(filepath.Join(s.Root, "deleted-projects", id+".json"), d); err != nil {
		return d, err
	}
	if err = s.archiveDeletedProjectLocked(id); err != nil {
		return d, fmt.Errorf("已阻止该项目同步，移入回收目录失败：%w", err)
	}
	return d, nil
}

// Caller holds mu; make archived local edits recoverable in the same backup UI.
func (s *Store) checkpointDeletedProject(snap SyncSnapshot) error {
	id := snap.Project.ID
	var err error
	// Durable portable checkpoint before recording deletion. Failure leaves the project intact.
	dir := filepath.Join(s.Root, "daily-backups", ID())
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err = writeSyncJSON(filepath.Join(dir, id+".json"), snap.Project); err != nil {
		return err
	}
	backupID := filepath.Base(dir)
	if err = writeSyncJSON(filepath.Join(dir, "manifest.json"), DailyBackup{Kind: "before-delete", ID: backupID, CreatedAt: Now(), Projects: []BackupProject{{id, snap.Project.Name}}}); err != nil {
		return err
	}
	return nil
}

// Caller holds mu. Moving the entire DB directory also preserves WAL and history.
func (s *Store) archiveDeletedProjectLocked(id string) error {
	if !validID.MatchString(id) {
		return errors.New("项目ID无效")
	}
	s.projectFilesMu.Lock()
	defer s.projectFilesMu.Unlock()
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	for _, name := range []string{"projects", "deleted-project-backups"} {
		dir := filepath.Join(root, name)
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		info, e := os.Lstat(dir)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("项目或回收目录不是普通目录")
		}
	}
	source := filepath.Join(root, "projects", id)
	info, err := os.Lstat(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("项目不是普通目录")
	}
	return os.Rename(source, filepath.Join(root, "deleted-project-backups", id+"-"+ID()))
}
func (s *Store) archiveUnchangedDeleted(id, expected string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, err := s.syncSnapshot(id)
	if err != nil {
		return err
	}
	if snap.Hash != expected {
		return ErrConflict
	}
	if err = s.checkpointDeletedProject(snap); err != nil {
		return err
	}
	return s.archiveDeletedProjectLocked(id)
}
func (s *Store) ResolveDeletedProject(id, expected string, keep bool) (Project, error) {
	if !s.syncRunMu.TryLock() {
		return Project{}, errors.New("同步正在进行")
	}
	defer s.syncRunMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, err := s.syncSnapshot(id)
	if err != nil {
		return Project{}, err
	}
	if expected == "" || snap.Hash != expected {
		return Project{}, ErrConflict
	}
	var copy Project
	if keep {
		copy = snap.Project
		copy.ID = ID()
		copy.Name += " · 保留副本"
		copy.Revision = 1
		created, e := s.applySyncLocked(copy, "")
		if e != nil {
			return Project{}, e
		}
		copy = created.Project
		latest, e := s.syncSnapshot(id)
		if e != nil {
			return copy, e
		}
		if latest.Hash != expected {
			return copy, ErrConflict
		}
	}
	if err = s.checkpointDeletedProject(snap); err != nil {
		return copy, err
	}
	err = s.archiveDeletedProjectLocked(id)
	return copy, err
}
func (s *Server) registerProjectDeletion(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{id}/deletion-check", func(w http.ResponseWriter, r *http.Request) {
		snap, err := s.Store.syncSnapshot(r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]string{"hash": snap.Hash})
	})
	mux.HandleFunc("DELETE /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !s.Remote {
			fail(w, errors.New("全局删除请在服务端内网工作台执行"))
			return
		}
		var in struct {
			Hash string `json:"hash"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		d, err := s.Store.DeleteServerProject(r.PathValue("id"), in.Hash)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, d)
	})
	mux.HandleFunc("POST /api/projects/{id}/resolve-deleted", func(w http.ResponseWriter, r *http.Request) {
		if s.Remote {
			fail(w, errors.New("请在客户端处理删除通知"))
			return
		}
		var in struct {
			Hash string `json:"hash"`
			Keep bool   `json:"keep"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		p, err := s.Store.ResolveDeletedProject(r.PathValue("id"), in.Hash, in.Keep)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, p)
	})
}
