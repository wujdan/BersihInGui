package quarantine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"storage-optimizer/internal/config"
	"storage-optimizer/internal/models"
)

// Manifest records one quarantined file.
type Manifest struct {
	ID           string                 `json:"id"`
	OriginalPath string                 `json:"original_path"`
	QuarantinePath string              `json:"quarantine_path"`
	Size         int64                  `json:"size"`
	SHA256       string                 `json:"sha256"`
	MovedAt      time.Time              `json:"moved_at"`
	RetentionUntil time.Time            `json:"retention_until"`
	Category     string                 `json:"category"`
	Label        string                 `json:"label"`
}

// Dir manages the quarantine storage area.
type Dir struct {
	mu      sync.Mutex
	root    string
	cfg     *config.Config
	blobDir string
	logDir  string
	manifests map[string]*Manifest // by id
}

// New creates/opens a quarantine directory.
func New(cfg *config.Config) (*Dir, error) {
	root := cfg.App.DataDir
	q := &Dir{
		root:       root,
		cfg:        cfg,
		blobDir:    filepath.Join(root, "quarantine"),
		logDir:     filepath.Join(root, "logs"),
		manifests:  make(map[string]*Manifest),
	}
	for _, d := range []string{q.blobDir, q.logDir, filepath.Join(root, "reports")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", d, err)
		}
	}
	if err := q.loadManifests(); err != nil {
		return nil, err
	}
	return q, nil
}

// manifestsFile returns path of the manifest store.
func (q *Dir) manifestsFile() string {
	return filepath.Join(q.root, "manifest.json")
}

// loadManifests reads existing manifests.
func (q *Dir) loadManifests() error {
	data, err := os.ReadFile(q.manifestsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var list []*Manifest
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	for _, m := range list {
		q.manifests[m.ID] = m
	}
	return nil
}

// persist saves the manifest to disk atomically.
func (q *Dir) persist() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	list := make([]*Manifest, 0, len(q.manifests))
	for _, m := range q.manifests {
		list = append(list, m)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := q.manifestsFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, q.manifestsFile())
}

// Persist writes the current manifest list to disk. Call it once after
// a batch of moves (instead of once per file) to avoid O(n²) serialization.
func (q *Dir) Persist() error {
	return q.persist()
}

// retentionFor returns the retention days for a category (tahap 10 tiers).
func (q *Dir) retentionFor(cat models.Category) int {
	switch cat {
	case models.CategoryCache:
		return q.cfg.Retention.CacheTemp
	case models.CategoryLog, models.CategoryDuplicate:
		return q.cfg.Retention.LogDup
	default:
		return q.cfg.Retention.LargeOld
	}
}

// Staged is a candidate that has been hashed and is ready to be committed.
// Staging is cheap and safe to run concurrently; Commit must be called
// serially because parallel renames on the same NTFS volume are ~10x slower.
type Staged struct {
	cand *models.Candidate
	id   string
	dst  string
	sum  string
}

// Stage computes the SHA-256 and target path without touching the filesystem.
func (q *Dir) Stage(c *models.Candidate, blobDir string, seq *int64) (*Staged, error) {
	sum, err := hashFile(c.Meta.Path)
	if err != nil {
		return nil, fmt.Errorf("hash %s: %w", c.Meta.Path, err)
	}
	id := newID(sum, seq)
	dst := filepath.Join(blobDir, id+"_"+sanitizeName(c.Meta.Name()))
	return &Staged{cand: c, id: id, dst: dst, sum: sum}, nil
}

// Candidate returns the candidate being committed.
func (s *Staged) Candidate() *models.Candidate { return s.cand }

// Commit performs the rename and records the manifest.
func (q *Dir) Commit(s *Staged) (*Manifest, error) {
	if err := os.Rename(s.cand.Meta.Path, s.dst); err != nil {
		// Windows cannot rename across drives; fall back to copy+delete.
		if err2 := copyMove(s.cand.Meta.Path, s.dst); err2 != nil {
			return nil, fmt.Errorf("move %s -> %s: %w", s.cand.Meta.Path, s.dst, err2)
		}
	}
	m := &Manifest{
		ID:             s.id,
		OriginalPath:   s.cand.Meta.Path,
		QuarantinePath: s.dst,
		Size:           s.cand.Meta.Size,
		SHA256:         s.sum,
		MovedAt:        time.Now(),
		RetentionUntil: time.Now().AddDate(0, 0, q.retentionFor(s.cand.Category)),
		Category:       string(s.cand.Category),
		Label:          string(s.cand.Label),
	}
	q.mu.Lock()
	q.manifests[s.id] = m
	q.mu.Unlock()
	return m, nil
}

// MoveFile moves a file into a prepared blobDir and records it in the manifest.
// It computes SHA-256 before the move (audit + integrity) and uses seq to build
// collision-free ids when many files (including duplicates) move in one second.
func (q *Dir) MoveFile(c *models.Candidate, blobDir string, seq *int64) (*Manifest, error) {
	s, err := q.Stage(c, blobDir, seq)
	if err != nil {
		return nil, err
	}
	return q.Commit(s)
}

// newID builds a unique quarantine id: timestamp + sequence + sha prefix.
// Using an atomically-incremented sequence guarantees uniqueness even when
// duplicate files (identical sha256) are moved within the same second.
func newID(sum string, seq *int64) string {
	var n int64
	if seq != nil {
		n = atomic.AddInt64(seq, 1)
	}
	return fmt.Sprintf("%s-%05d-%s", time.Now().Format("20060102150405"), n, sum[:8])
}

// BlobDirFor returns (and creates) the blob directory for a given batch.
// Call it once before a batch instead of per file to avoid repeated MkdirAll.
func (q *Dir) BlobDirFor(day string) (string, error) {
	dir := filepath.Join(q.blobDir, day)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Restore moves a quarantined file back to its original location.
func (q *Dir) Restore(id string) (*Manifest, error) {
	q.mu.Lock()
	m := q.manifests[id]
	q.mu.Unlock()
	if m == nil {
		return nil, fmt.Errorf("file %q tidak ditemukan di quarantine", id)
	}
	if err := os.MkdirAll(filepath.Dir(m.OriginalPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.Rename(m.QuarantinePath, m.OriginalPath); err != nil {
		if err2 := copyMove(m.QuarantinePath, m.OriginalPath); err2 != nil {
			return nil, fmt.Errorf("restore %s: %w", m.OriginalPath, err2)
		}
	}
	q.mu.Lock()
	delete(q.manifests, id)
	q.mu.Unlock()
	if err := q.persist(); err != nil {
		return m, err
	}
	return m, nil
}

// PurgeExpired permanently removes files past their retention date.
// Returns count + bytes freed. If ids is non-empty, only those are purged.
func (q *Dir) PurgeExpired(ids []string, force bool) (int, int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	toDelete := make([]*Manifest, 0)
	if len(ids) > 0 {
		for _, id := range ids {
			if m := q.manifests[id]; m != nil {
				toDelete = append(toDelete, m)
			}
		}
	} else {
		for _, m := range q.manifests {
			if force || now.After(m.RetentionUntil) {
				toDelete = append(toDelete, m)
			}
		}
	}

	var freed int64
	var count int
	for _, m := range toDelete {
		if err := os.Remove(m.QuarantinePath); err != nil && !os.IsNotExist(err) {
			continue
		}
		freed += m.Size
		count++
		delete(q.manifests, m.ID)
	}
	if count > 0 {
		if err := q.persistLocked(); err != nil {
			return count, freed, err
		}
	}
	return count, freed, nil
}

// List returns all quarantined manifests.
func (q *Dir) List() []*Manifest {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]*Manifest, 0, len(q.manifests))
	for _, m := range q.manifests {
		out = append(out, m)
	}
	// sort by moved timestamp desc
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].MovedAt.After(out[j-1].MovedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Count returns how many files are currently quarantined.
func (q *Dir) Count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.manifests)
}

// Get returns a single manifest by id.
func (q *Dir) Get(id string) (*Manifest, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	m, ok := q.manifests[id]
	return m, ok
}

// Root returns the quarantine directory root.
func (q *Dir) Root() string { return q.blobDir }

// LogDir returns the log directory.
func (q *Dir) LogDir() string { return q.logDir }

// persistLocked persists assuming the mutex is already held.
func (q *Dir) persistLocked() error {
	list := make([]*Manifest, 0, len(q.manifests))
	for _, m := range q.manifests {
		list = append(list, m)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := q.manifestsFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, q.manifestsFile())
}

// copyMove moves a file across volumes by copying to a temp name in the
// destination directory, removing the source, then renaming to the final
// name. On failure the partial copy is cleaned up so no data is lost.
func copyMove(src, dst string) error {
	part := dst + ".part"
	if err := copyFile(src, part); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		_ = os.Remove(part)
		return err
	}
	return os.Rename(part, dst)
}

// copyFile copies src to dst preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := in.WriteTo(out); err != nil {
		_ = os.Remove(dst)
		return err
	}
	if info, serr := os.Stat(src); serr == nil {
		_ = os.Chmod(dst, info.Mode())
	}
	return nil
}

// hashFile computes SHA-256 of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 256*1024)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if rerr != nil {
			break
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sanitizeName strips path separators from a file name.
func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	return name
}