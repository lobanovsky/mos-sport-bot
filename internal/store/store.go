package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type file struct {
	ChatIDs []int64 `json:"chat_ids"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	ids  map[int64]struct{}
}

// New loads the subscriber set from path, creating an empty store if the
// file does not exist yet.
func New(path string) (*Store, error) {
	s := &Store{path: path, ids: make(map[int64]struct{})}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	for _, id := range f.ChatIDs {
		s.ids[id] = struct{}{}
	}
	return s, nil
}

// Add subscribes chatID, persisting the change. It returns true if chatID
// was not already subscribed.
func (s *Store) Add(chatID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ids[chatID]; ok {
		return false, nil
	}
	next := s.snapshotLocked()
	next[chatID] = struct{}{}
	if err := s.persist(next); err != nil {
		return false, err
	}
	s.ids = next
	return true, nil
}

// Remove unsubscribes chatID, persisting the change. It returns true if
// chatID was subscribed.
func (s *Store) Remove(chatID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ids[chatID]; !ok {
		return false, nil
	}
	next := s.snapshotLocked()
	delete(next, chatID)
	if err := s.persist(next); err != nil {
		return false, err
	}
	s.ids = next
	return true, nil
}

// List returns a snapshot of the subscribed chat IDs.
func (s *Store) List() []int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]int64, 0, len(s.ids))
	for id := range s.ids {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (s *Store) snapshotLocked() map[int64]struct{} {
	next := make(map[int64]struct{}, len(s.ids)+1)
	for id := range s.ids {
		next[id] = struct{}{}
	}
	return next
}

// persist atomically writes ids to s.path via a temp file + rename in the
// same directory, so a crash mid-write can never leave a truncated file.
func (s *Store) persist(ids map[int64]struct{}) error {
	list := make([]int64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file{ChatIDs: list}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".subscribers-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
