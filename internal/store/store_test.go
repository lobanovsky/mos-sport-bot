package store

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestAddRemoveRoundTrip(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "subscribers.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	added, err := s.Add(1)
	if err != nil || !added {
		t.Fatalf("Add(1) = %v, %v, want true, nil", added, err)
	}
	added, err = s.Add(1)
	if err != nil || added {
		t.Fatalf("Add(1) again = %v, %v, want false, nil", added, err)
	}
	if got := s.List(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("List() = %v, want [1]", got)
	}
	removed, err := s.Remove(2)
	if err != nil || removed {
		t.Fatalf("Remove(2) = %v, %v, want false, nil", removed, err)
	}
	removed, err = s.Remove(1)
	if err != nil || !removed {
		t.Fatalf("Remove(1) = %v, %v, want true, nil", removed, err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("List() = %v, want empty", got)
	}
}

func TestPersistAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscribers.json")
	a, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Add(10); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := a.Add(20); err != nil {
		t.Fatalf("Add: %v", err)
	}

	b, err := New(path)
	if err != nil {
		t.Fatalf("New (reload): %v", err)
	}
	got := b.List()
	if len(got) != 2 || got[0] != 10 || got[1] != 20 {
		t.Fatalf("List() after reload = %v, want [10 20]", got)
	}
}

func TestNewWithMissingFile(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("List() = %v, want empty", got)
	}
}

func TestConcurrentAdd(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "subscribers.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var wg sync.WaitGroup
	for i := int64(0); i < 50; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, _ = s.Add(id)
		}(i)
	}
	wg.Wait()
	if got := s.List(); len(got) != 50 {
		t.Fatalf("List() has %d ids, want 50", len(got))
	}
}
