package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"safeops/internal/core"
	"time"
)

type Options struct{ Driver, Path, URL, Workspace string }

func OpenDatabase(ctx context.Context, o Options) (*Store, error) {
	if !core.ValidWorkspaceID(o.Workspace) {
		return nil, errors.New("valid workspace partition required")
	}
	var driver, dsn string
	switch o.Driver {
	case "sqlite":
		if !filepath.IsAbs(o.Path) || o.URL != "" {
			return nil, errors.New("SQLite requires an absolute file path and no database URL")
		}
		if e := os.MkdirAll(filepath.Dir(o.Path), 0700); e != nil {
			return nil, e
		}
		if parent, e := os.Stat(filepath.Dir(o.Path)); e != nil || parent.Mode().Perm()&0022 != 0 {
			return nil, errors.New("SQLite parent directory must not be writable by group or others")
		}
		if info, e := os.Lstat(o.Path); e == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
			return nil, errors.New("SQLite database must be a regular private file (0600)")
		}
		f, e := os.OpenFile(o.Path, os.O_CREATE|os.O_RDWR, 0600)
		if e != nil {
			return nil, e
		}
		f.Close()
		u := url.URL{Scheme: "file", Path: o.Path}
		q := url.Values{}
		q.Add("_pragma", "busy_timeout(5000)")
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "synchronous(FULL)")
		q.Add("_pragma", "foreign_keys(ON)")
		q.Set("_txlock", "immediate")
		u.RawQuery = q.Encode()
		driver, dsn = "sqlite", u.String()
	case "postgres":
		if o.URL == "" || o.Path != "" {
			return nil, errors.New("PostgreSQL requires a database URL and no SQLite path")
		}
		driver, dsn = "pgx", o.URL
	default:
		return nil, errors.New("database driver must be sqlite or postgres")
	}
	db, e := sql.Open(driver, dsn)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()
	db.SetMaxOpenConns(8)
	if driver == "sqlite" {
		db.SetMaxOpenConns(1)
	}
	db.SetConnMaxLifetime(30 * time.Minute)
	if e = db.PingContext(ctx); e != nil {
		return nil, fmt.Errorf("database connection failed: %w", e)
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if o.Driver == "postgres" {
		if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1396786757)`); e != nil {
			return nil, e
		}
	}
	// Transactional, additive schema migration. A newer schema fails closed.
	if _, e = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS safeops_schema (id INTEGER PRIMARY KEY CHECK(id=1), version INTEGER NOT NULL)`); e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO safeops_schema(id,version) VALUES(1,1) ON CONFLICT DO NOTHING`); e != nil {
		return nil, e
	}
	var version int
	if e = tx.QueryRowContext(ctx, `SELECT version FROM safeops_schema WHERE id=1`).Scan(&version); e != nil {
		return nil, e
	}
	if version != 1 {
		return nil, errors.New("unsupported database schema; upgrade binary or restore compatible backup")
	}
	if _, e = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS safeops_workspaces (id TEXT PRIMARY KEY, document TEXT NOT NULL)`); e != nil {
		return nil, e
	}
	state := core.State{Requests: []core.Request{}, Audit: []core.Audit{}, Heartbeats: map[string]int64{}}
	if _, e = tx.ExecContext(ctx, `INSERT INTO safeops_workspaces(id,document) VALUES($1,$2) ON CONFLICT DO NOTHING`, o.Workspace, string(core.JSON(state))); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	s := &Store{db: db, driver: o.Driver, workspace: o.Workspace}
	if e = s.Update(ctx, func(*core.State) error { return nil }); e != nil {
		return nil, e
	}
	ok = true
	return s, nil
}
func (s *Store) Ping(ctx context.Context) error {
	if s.db != nil {
		return s.db.PingContext(ctx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.poisoned
}

type Backup struct {
	Version   int        `json:"version"`
	Workspace string     `json:"workspace"`
	CreatedAt int64      `json:"created_at"`
	State     core.State `json:"state"`
	Digest    string     `json:"digest"`
}

func (s *Store) Backup(ctx context.Context) (Backup, error) {
	out := Backup{Version: 1, Workspace: s.workspace, CreatedAt: time.Now().Unix()}
	e := s.Update(ctx, func(st *core.State) error { return json.Unmarshal(core.JSON(st), &out.State) })
	out.Digest = core.Hash(core.JSON(out.State))
	return out, e
}

// Restore only into a new partition. Active requests are invalidated. Agent
// replay ledgers are never restored backwards alongside a control-plane backup.
func (s *Store) Restore(ctx context.Context, b Backup) error {
	if b.Version != 1 || b.Workspace != s.workspace || core.Hash(core.JSON(b.State)) != b.Digest {
		return errors.New("backup version, workspace or digest mismatch")
	}
	if e := VerifyAudit(b.State.Audit); e != nil {
		return e
	}
	return s.Update(ctx, func(st *core.State) error {
		if st.PolicyHash != "" || len(st.Requests) > 0 || len(st.Audit) > 0 {
			return errors.New("restore requires an empty workspace partition")
		}
		*st = b.State
		for n := range st.Requests {
			r := &st.Requests[n]
			switch r.Status {
			case "pending", "approved", "ready", "running":
				r.Status = "revoked"
				r.Queued = false
				if len(r.Executions) > 0 && r.Executions[len(r.Executions)-1].Status == "running" {
					r.Executions[len(r.Executions)-1].Status = "revoked"
				}
			}
		}
		st.Heartbeats = map[string]int64{}
		Append(st, time.Now(), "system", "backup.restored", "", "active grants revoked; verify current policy before starting")
		return nil
	})
}
