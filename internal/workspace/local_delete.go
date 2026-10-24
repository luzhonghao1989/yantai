package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type LocalDeleteResult struct {
	ID         string `json:"id"`
	BackupPath string `json:"backupPath"`
}

// Move the whole project directory (including WAL/SHM) without opening its DB.
// Never touch shared images or send a deletion to a remote peer.
func (s *Store) DeleteLocalProject(id string, draft *Project) (LocalDeleteResult, error) {
	result := LocalDeleteResult{ID: id}
	if !validID.MatchString(id) {
		return result, errors.New("项目 ID 无效")
	}
	if draft != nil && draft.ID != id {
		return result, errors.New("草稿项目 ID 不匹配")
	}
	if !s.syncRunMu.TryLock() {
		return result, errors.New("同步正在进行，请结束后再删除本机项目")
	}
	defer s.syncRunMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	// Wait until readers and their SQLite connections are closed before moving
	// directories, so an old connection cannot touch the replacement's WAL files.
	s.projectFilesMu.Lock()
	defer s.projectFilesMu.Unlock()
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	var state syncState
	statePath := filepath.Join(s.Root, "sync-state.json")
	if err := readSyncJSON(statePath, &state); err != nil {
		return result, fmt.Errorf("同步状态无法读取，未删除项目：%w", err)
	}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return result, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return result, err
	}
	projectsInfo, err := os.Lstat(filepath.Join(root, "projects"))
	if err != nil || !projectsInfo.IsDir() || projectsInfo.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("项目根目录不是普通目录，未删除")
	}
	source := filepath.Join(root, "projects", id)
	info, err := os.Lstat(source)
	if err != nil {
		return result, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("项目路径不是普通目录，未删除")
	}
	archive := filepath.Join(root, "local-project-backups")
	if err = os.MkdirAll(archive, 0700); err != nil {
		return result, err
	}
	archiveInfo, err := os.Lstat(archive)
	if err != nil || !archiveInfo.IsDir() || archiveInfo.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("本机备份目录不可用，未删除")
	}
	target := filepath.Join(archive, id+"-"+ID())
	if err = os.Rename(source, target); err != nil {
		return result, fmt.Errorf("无法保留本机备份，未删除：%w", err)
	}
	rollback := func(cause error) (LocalDeleteResult, error) {
		if restoreErr := os.Rename(target, source); restoreErr != nil {
			return result, fmt.Errorf("删除未完成：%v；原件已保留在 %s，恢复目录失败：%w", cause, target, restoreErr)
		}
		return result, fmt.Errorf("删除未完成，已恢复原项目目录：%w", cause)
	}
	if draft != nil {
		body, e := json.MarshalIndent(draft, "", "  ")
		if e != nil {
			return rollback(e)
		}
		if e = os.WriteFile(filepath.Join(target, "unsaved-draft-"+ID()+".json"), body, 0600); e != nil {
			return rollback(e)
		}
	}
	// Reset only this ID for every peer. Next sync can download its remote copy,
	// while baselines for all other novels remain untouched.
	for _, peer := range state.Peers {
		delete(peer, id)
	}
	if state.Last != nil {
		items := state.Last.Items[:0]
		for _, item := range state.Last.Items {
			if item.ID != id {
				items = append(items, item)
			}
		}
		state.Last.Items = items
	}
	if err = writeSyncJSON(statePath, state); err != nil {
		return rollback(err)
	}
	result.BackupPath = target
	return result, nil
}
