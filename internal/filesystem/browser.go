// Package filesystem lists host directory metadata for the authenticated owner.
// It exposes no content reads, uploads, mutations, or model tool surface.
package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const pageSize = 250
const maxEntries = 50000
const maxSnapshots = 8
const maxListingBytes = 8 * 1024 * 1024
const snapshotLifetime = 2 * time.Minute

var ErrCursor = errors.New("directory listing expired or changed; refresh this folder")
var ErrPath = errors.New("choose an absolute directory path on the daemon host")
var ErrKind = errors.New("kind must be directory, file, or any")
var ErrTooLarge = errors.New("directory listing is too large; enter a more specific folder path")

type Entry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Selectable bool   `json:"selectable"`
}
type Listing struct {
	Path       string  `json:"path"`
	Parent     *string `json:"parent"`
	Entries    []Entry `json:"entries"`
	NextCursor *string `json:"next_cursor"`
}
type Query struct {
	Path, Kind, Cursor string
	Hidden             bool
}
type snapshot struct {
	query   Query
	entries []Entry
	created time.Time
}

// Browser keeps bounded, short-lived snapshots so pagination is stable even if
// a directory changes between pages. Initial requests always read fresh metadata.
type Browser struct {
	scans     chan struct{}
	home      string
	mu        sync.Mutex
	snapshots map[string]snapshot
	now       func() time.Time
}

func New(home string) *Browser {
	return &Browser{home: home, snapshots: make(map[string]snapshot), now: time.Now, scans: make(chan struct{}, 2)}
}

func (b *Browser) List(ctx context.Context, q Query) (Listing, error) {
	if q.Kind == "" {
		q.Kind = "any"
	}
	if q.Kind != "directory" && q.Kind != "file" && q.Kind != "any" {
		return Listing{}, ErrKind
	}
	if q.Path == "" {
		q.Path = b.home
	}
	if !filepath.IsAbs(q.Path) || len(q.Path) > 8192 || strings.ContainsRune(q.Path, 0) {
		return Listing{}, ErrPath
	}
	q.Path = filepath.Clean(q.Path)
	if err := ctx.Err(); err != nil {
		return Listing{}, err
	}
	if q.Cursor != "" {
		return b.next(q)
	}
	select {
	case b.scans <- struct{}{}:
		defer func() { <-b.scans }()
	case <-ctx.Done():
		return Listing{}, ctx.Err()
	}
	path, err := filepath.EvalSymlinks(q.Path)
	if err != nil {
		return Listing{}, err
	}
	q.Path = path
	entries, err := readDirectory(ctx, q)
	if err != nil {
		return Listing{}, err
	}
	id := ""
	if len(entries) > pageSize {
		var key [16]byte
		if _, err = rand.Read(key[:]); err != nil {
			return Listing{}, err
		}
		id = hex.EncodeToString(key[:])
		b.mu.Lock()
		now := b.now()
		for key, s := range b.snapshots {
			if now.Sub(s.created) >= snapshotLifetime {
				delete(b.snapshots, key)
			}
		}
		for len(b.snapshots) >= maxSnapshots {
			oldest := ""
			var earliest time.Time
			for key, s := range b.snapshots {
				if oldest == "" || s.created.Before(earliest) {
					oldest, earliest = key, s.created
				}
			}
			delete(b.snapshots, oldest)
		}
		b.snapshots[id] = snapshot{query: q, entries: entries, created: now}
		b.mu.Unlock()
	}
	return page(q.Path, entries, id, 0), nil
}
func (b *Browser) next(q Query) (Listing, error) {
	id, offsetText, ok := strings.Cut(q.Cursor, ":")
	offset, err := strconv.Atoi(offsetText)
	if !ok || err != nil || offset < pageSize || offset%pageSize != 0 {
		return Listing{}, ErrCursor
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.snapshots[id]
	if !ok {
		return Listing{}, ErrCursor
	}
	if b.now().Sub(s.created) >= snapshotLifetime {
		delete(b.snapshots, id)
		return Listing{}, ErrCursor
	}
	if q.Path != s.query.Path || q.Hidden != s.query.Hidden || q.Kind != s.query.Kind || offset >= len(s.entries) {
		return Listing{}, ErrCursor
	}
	return page(q.Path, s.entries, id, offset), nil
}
func page(path string, entries []Entry, id string, offset int) Listing {
	end := min(offset+pageSize, len(entries))
	out := Listing{Path: path, Entries: append([]Entry{}, entries[offset:end]...)}
	parent := filepath.Dir(path)
	if parent != path {
		out.Parent = &parent
	}
	if end < len(entries) {
		cursor := id + ":" + strconv.Itoa(end)
		out.NextCursor = &cursor
	}
	return out
}
func readDirectory(ctx context.Context, q Query) ([]Entry, error) {
	info, err := os.Stat(q.Path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrPath
	}
	// OpenRoot opens directories only, including if the path is replaced after
	// Stat. A FIFO or device path must never block a listing by being opened.
	root, err := os.OpenRoot(q.Path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries := []Entry{}
	listingBytes := 0
	scanned := 0
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		batch, readErr := dir.ReadDir(256)
		scanned += len(batch)
		if scanned > maxEntries {
			return nil, ErrTooLarge
		}
		for _, item := range batch {
			if !q.Hidden && strings.HasPrefix(item.Name(), ".") {
				continue
			}
			// Only directories and regular files are selectable; sockets, devices,
			// pipes and broken links never enter the picker. Symlinks resolve on entry.
			mode := item.Type()
			if mode&os.ModeSymlink != 0 {
				target, e := os.Stat(filepath.Join(q.Path, item.Name()))
				if e != nil {
					continue
				}
				mode = target.Mode()
			}
			kind := "file"
			if mode.IsDir() {
				kind = "directory"
			} else if !mode.IsRegular() {
				continue
			}
			if q.Kind == "directory" && kind != "directory" {
				continue
			}
			entryPath := filepath.Join(q.Path, item.Name())
			listingBytes += len(item.Name()) + len(entryPath) + 64
			if listingBytes > maxListingBytes {
				return nil, ErrTooLarge
			}
			entries = append(entries, Entry{Name: item.Name(), Path: entryPath, Kind: kind, Selectable: q.Kind == "any" || q.Kind == kind})
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		a, z := entries[i], entries[j]
		if a.Kind != z.Kind {
			return a.Kind == "directory"
		}
		lowerA, lowerZ := strings.ToLower(a.Name), strings.ToLower(z.Name)
		if lowerA != lowerZ {
			return lowerA < lowerZ
		}
		return a.Name < z.Name
	})
	return entries, nil
}
