package quarantine

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"storage-optimizer/internal/config"
	"storage-optimizer/internal/models"
)

func TestMoveFileUsesPreparedBlobDir(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Default()
	cfg.App.DataDir = filepath.Join(tmp, "data")

	q, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "cache_1.tmp")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	blobDir, err := q.BlobDirFor("2026-01-01")
	if err != nil {
		t.Fatal(err)
	}

	var seq int64
	c := &models.Candidate{
		Meta:     &models.FileMeta{Path: src, Size: 5},
		Category: models.CategoryCache,
		Label:    "cache",
	}
	m, err := q.MoveFile(c, blobDir, &seq)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.QuarantinePath); err != nil {
		t.Fatalf("quarantined file missing: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be gone, got err=%v", err)
	}
	if got := q.Count(); got != 1 {
		t.Fatalf("count=%d want 1", got)
	}
	t.Cleanup(func() { _ = q.Persist() })
}

func TestNewIDUniqueForDuplicates(t *testing.T) {
	sum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var seq int64

	seen := make(map[string]bool)
	for i := 0; i < 100000; i++ {
		id := newID(sum, &seq)
		if seen[id] {
			t.Fatalf("duplicate id after %d moves: %s", i, id)
		}
		seen[id] = true
	}
	// without a sequence pointer the ids share the timestamp+sha prefix;
	// with one they must be strictly increasing and unique
	if atomic.LoadInt64(&seq) != 100000 {
		t.Fatalf("seq=%d want 100000", seq)
	}
}