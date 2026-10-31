package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"safeops/internal/core"
)

// Every transition and its audit event commit together. PostgreSQL serializes
// writers across processes; demo files hold an exclusive lifetime lock.
type Store struct {
	mu        sync.Mutex
	path      string
	db        *sql.DB
	driver    string
	workspace string
	lock      *os.File
	state     core.State
	poisoned  error
}

func Open(ctx context.Context, path, databaseURL string) (*Store, error) {
	s := &Store{path: path, state: core.State{Requests: []core.Request{}, Audit: []core.Audit{}, Heartbeats: map[string]int64{}}}
	if databaseURL != "" {
		db, e := sql.Open("pgx", databaseURL)
		if e != nil {
			return nil, e
		}
		s.db = db
		db.SetMaxOpenConns(8)
		if _, e = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS safeops_state (id INTEGER PRIMARY KEY CHECK(id = 1), document JSONB NOT NULL)`); e != nil {
			db.Close()
			return nil, e
		}
		if _, e = db.ExecContext(ctx, `INSERT INTO safeops_state (id,document) VALUES(1,$1) ON CONFLICT DO NOTHING`, core.JSON(s.state)); e != nil {
			db.Close()
			return nil, e
		}
		return s, nil
	}
	if path == "" {
		return s, nil
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("state already in use: %w", e)
	}
	s.lock = f
	b, e := os.ReadFile(path)
	if e == nil {
		e = json.Unmarshal(b, &s.state)
	}
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		s.Close()
		return nil, e
	}
	if s.state.Heartbeats == nil {
		s.state.Heartbeats = map[string]int64{}
	}
	if e = VerifyAudit(s.state.Audit); e != nil {
		s.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	if s.lock != nil {
		return s.lock.Close()
	}
	return nil
}
func (s *Store) Update(ctx context.Context, fn func(*core.State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.poisoned != nil {
		return s.poisoned
	}
	var tx *sql.Tx
	current := s.state
	if s.db != nil {
		var e error
		tx, e = s.db.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		var b []byte
		query := `SELECT document FROM safeops_state WHERE id=1 FOR UPDATE`
		args := []any{}
		if s.workspace != "" {
			query = `SELECT document FROM safeops_workspaces WHERE id=$1`
			args = []any{s.workspace}
			if s.driver == "postgres" {
				query += ` FOR UPDATE`
			}
		}
		if e = tx.QueryRowContext(ctx, query, args...).Scan(&b); e != nil {
			return e
		}
		if e = json.Unmarshal(b, &current); e != nil {
			return e
		}
	}
	// Clone even in memory so a failed callback cannot leave partial mutations.
	var next core.State
	if e := json.Unmarshal(core.JSON(current), &next); e != nil {
		return e
	}
	if e := VerifyAudit(next.Audit); e != nil {
		return e
	}
	if next.Heartbeats == nil {
		next.Heartbeats = map[string]int64{}
	}
	if e := fn(&next); e != nil {
		return e
	}
	if tx != nil {
		query := `UPDATE safeops_state SET document=$1 WHERE id=1`
		args := []any{core.JSON(next)}
		if s.workspace != "" {
			query = `UPDATE safeops_workspaces SET document=$1 WHERE id=$2`
			args = []any{string(core.JSON(next)), s.workspace}
		}
		if _, e := tx.ExecContext(ctx, query, args...); e != nil {
			return e
		}
		return tx.Commit()
	}
	if s.path != "" {
		if e := AtomicWrite(s.path, core.JSON(next)); e != nil {
			s.poisoned = fmt.Errorf("persistence failed; restart required: %w", e)
			return s.poisoned
		}
	}
	s.state = next
	return nil
}
func AtomicWrite(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".safeops-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func Append(st *core.State, now time.Time, actor, event, id, detail string) {
	a := core.Audit{Sequence: len(st.Audit) + 1, At: now.Unix(), Actor: actor, Event: event, RequestID: id, Detail: detail}
	if len(st.Audit) > 0 {
		a.Previous = st.Audit[len(st.Audit)-1].Hash
	}
	a.Hash = core.Hash(core.JSON(a))
	st.Audit = append(st.Audit, a)
}
func VerifyAudit(events []core.Audit) error {
	previous := ""
	for i, a := range events {
		h := a.Hash
		a.Hash = ""
		if a.Sequence != i+1 || a.Previous != previous || core.Hash(core.JSON(a)) != h {
			return fmt.Errorf("audit chain mismatch at event %d", i+1)
		}
		previous = h
	}
	return nil
}
