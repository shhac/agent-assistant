package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sync"
)

// Store serializes mutations in-process and uses BEGIN IMMEDIATE to serialize
// other processes. Each mutation atomically records entities and audit events.
type Store struct {
	db *sql.DB
	mu sync.Mutex
}
type diskState struct {
	Events     map[string]bool `json:"events"`
	Snapshot   Snapshot        `json:"snapshot"`
	ModelCalls map[string]int  `json:"model_calls"`
}

func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		f.Close()
		if err = os.Chmod(path, 0600); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; CREATE TABLE IF NOT EXISTS state (id INTEGER PRIMARY KEY CHECK(id=1), payload TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err = s.update(context.Background(), func(*Snapshot) error { return nil }); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func emptyState() Snapshot {
	return Snapshot{Projects: []Project{}, Agents: []Agent{}, Decisions: []Decision{}, Messages: []Message{}, Memories: []Memory{}, Activity: []Activity{}, Integrations: []Integration{}, Events: map[string]bool{}, ModelCalls: map[string]int{}}
}
func readState(ctx context.Context, conn *sql.Conn) (Snapshot, error) {
	var data string
	err := conn.QueryRowContext(ctx, "SELECT payload FROM state WHERE id=1").Scan(&data)
	if err == sql.ErrNoRows {
		return emptyState(), nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	d := diskState{Snapshot: emptyState()}
	if err = json.Unmarshal([]byte(data), &d); err != nil {
		return Snapshot{}, fmt.Errorf("decode durable state: %w", err)
	}
	d.Snapshot.Events = d.Events
	if d.Snapshot.Events == nil {
		d.Snapshot.Events = map[string]bool{}
	}
	d.Snapshot.ModelCalls = d.ModelCalls
	if d.Snapshot.ModelCalls == nil {
		d.Snapshot.ModelCalls = map[string]int{}
	}
	return d.Snapshot, nil
}
func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer conn.Close()
	return readState(ctx, conn)
}
func (s *Store) update(ctx context.Context, fn func(*Snapshot) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	state, err := readState(ctx, conn)
	if err != nil {
		return err
	}
	if err = fn(&state); err != nil {
		return err
	}
	data, err := json.Marshal(diskState{Snapshot: state, ModelCalls: state.ModelCalls, Events: state.Events})
	if err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, "INSERT INTO state(id,payload) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,version=version+1", string(data)); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}
