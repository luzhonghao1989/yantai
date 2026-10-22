package workspace

import (
	"database/sql"
	"encoding/json"
	"errors"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
)

var ErrConflict = errors.New("内容已被其他窗口更新，请导出当前草稿后重新打开项目")
var validID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Store struct {
	Root        string
	mu          sync.Mutex
	embeddingMu sync.Mutex
	embeddings  map[string][]float64
}
type ProjectInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Genre       string `json:"genre"`
	Description string `json:"description"`
	UpdatedAt   string `json:"updatedAt"`
	Words       int    `json:"words"`
	Chapters    int    `json:"chapters"`
}
type HistoryEntry struct {
	Revision  int64  `json:"revision"`
	CreatedAt string `json:"createdAt"`
	Name      string `json:"name"`
}

func NewStore(root string) (*Store, error) {
	err := os.MkdirAll(filepath.Join(root, "projects"), 0700)
	return &Store{Root: root}, err
}
func (s *Store) db(id string, create bool) (*sql.DB, error) {
	if !validID.MatchString(id) {
		return nil, errors.New("项目 ID 无效")
	}
	dir := filepath.Join(s.Root, "projects", id)
	if create {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	} else {
		if _, err := os.Stat(filepath.Join(dir, "novel.db")); err != nil {
			return nil, errors.New("项目不存在")
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "novel.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; CREATE TABLE IF NOT EXISTS state (id INTEGER PRIMARY KEY CHECK(id=1),revision INTEGER NOT NULL,body TEXT NOT NULL); CREATE TABLE IF NOT EXISTS history (revision INTEGER PRIMARY KEY,created_at TEXT NOT NULL,name TEXT NOT NULL,body TEXT NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func (s *Store) Create(name string, demo bool) (Project, error) {
	p := NewProject(name, demo)
	if err := Validate(p); err != nil {
		return p, err
	}
	db, err := s.db(p.ID, true)
	if err != nil {
		return p, err
	}
	defer db.Close()
	data, _ := json.Marshal(p)
	_, err = db.Exec(`INSERT INTO state VALUES(1,?,?)`, p.Revision, string(data))
	return p, err
}
func (s *Store) Load(id string) (Project, error) {
	var p Project
	db, err := s.db(id, false)
	if err != nil {
		return p, err
	}
	defer db.Close()
	var data string
	err = db.QueryRow(`SELECT body FROM state WHERE id=1`).Scan(&data)
	if err == nil {
		err = json.Unmarshal([]byte(data), &p)
	}
	return p, err
}
func (s *Store) Save(p Project) (Project, error) {
	if err := Validate(p); err != nil {
		return p, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	db, err := s.db(p.ID, false)
	if err != nil {
		return p, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	var rev int64
	var body string
	if err = tx.QueryRow(`SELECT revision,body FROM state WHERE id=1`).Scan(&rev, &body); err != nil {
		return p, err
	}
	if rev != p.Revision {
		return p, ErrConflict
	}
	if _, err = tx.Exec(`INSERT INTO history VALUES(?,?,?,?)`, rev, Now(), "自动保存", body); err != nil {
		return p, err
	}
	p.Revision++
	p.UpdatedAt = Now()
	data, _ := json.Marshal(p)
	if _, err = tx.Exec(`UPDATE state SET revision=?,body=? WHERE id=1`, p.Revision, string(data)); err != nil {
		return p, err
	}
	if _, err = tx.Exec(`DELETE FROM history WHERE revision NOT IN (SELECT revision FROM history ORDER BY revision DESC LIMIT 200)`); err != nil {
		return p, err
	}
	err = tx.Commit()
	return p, err
}
func (s *Store) List() ([]ProjectInfo, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "projects"))
	if err != nil {
		return nil, err
	}
	out := []ProjectInfo{}
	for _, e := range entries {
		if !e.IsDir() || !validID.MatchString(e.Name()) {
			continue
		}
		p, err := s.Load(e.Name())
		if err != nil {
			return nil, err
		}
		n := 0
		for _, c := range p.Chapters {
			n += WordCount(c.Text)
		}
		out = append(out, ProjectInfo{p.ID, p.Name, p.Genre, p.Description, p.UpdatedAt, n, len(p.Chapters)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}
func (s *Store) History(id string) ([]HistoryEntry, error) {
	db, err := s.db(id, false)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT revision,created_at,name FROM history ORDER BY revision DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var h HistoryEntry
		if err = rows.Scan(&h.Revision, &h.CreatedAt, &h.Name); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
func (s *Store) Restore(id string, revision, expected int64) (Project, error) {
	db, err := s.db(id, false)
	if err != nil {
		return Project{}, err
	}
	var data string
	err = db.QueryRow(`SELECT body FROM history WHERE revision=?`, revision).Scan(&data)
	db.Close()
	if err != nil {
		return Project{}, err
	}
	var p Project
	if err = json.Unmarshal([]byte(data), &p); err != nil {
		return p, err
	}
	p.Revision = expected
	return s.Save(p)
}
func WordCount(s string) int {
	n := 0
	for _, r := range s {
		if r > ' ' {
			n++
		}
	}
	return n
}
