package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"storage-optimizer/internal/config"
	"storage-optimizer/internal/models"
)

// HashSet stores all content hashes encountered (for dedup detection).
var HashSet = struct {
	sync.RWMutex
	m map[string]string
}{m: make(map[string]string)}

// Scanner walks directories and produces file metadata.
type Scanner struct {
	cfg     *config.Config
	excludes []string
	workers int
	roots   []string
}

// New creates a scanner with the given config.
func New(cfg *config.Config, roots []string) *Scanner {
	workers := cfg.Scan.WorkerCount
	if workers <= 0 {
		workers = runtime.NumCPU()
		if workers > 8 {
			workers = 8
		}
	}
	return &Scanner{
		cfg:      cfg,
		excludes: cfg.PlatformExcludes(),
		workers:  workers,
		roots:    roots,
	}
}

// Result is a single scanned file.
type Result struct {
	File models.FileMeta
	Err  error
}

// Scan walks the root directories and streams file metadata to ch.
// It respects the exclude list and handles errors gracefully.
func (s *Scanner) Scan(ctx context.Context, ch chan<- models.FileMeta) (filesScanned int64, dirsScanned int64, errs []string, err error) {
	var fileCount int64
	var dirCount int64

	// normalize absolute paths, report roots
	normalizedRoots := make([]string, 0, len(s.roots))
	for _, r := range s.roots {
		abs, aerr := filepath.Abs(r)
		if aerr != nil {
			errs = append(errs, fmt.Sprintf("resolve %s: %v", r, aerr))
			continue
		}
		if info, serr := os.Stat(abs); serr != nil {
			errs = append(errs, fmt.Sprintf("stat %s: %v", abs, serr))
			continue
		} else if !info.IsDir() {
			// single file root
			meta, merr := s.statFile(abs)
			if merr == nil {
				ch <- meta
				atomic.AddInt64(&fileCount, 1)
			}
			continue
		}
		normalizedRoots = append(normalizedRoots, abs)
	}

	// Walk all roots sequentially (directory reads are sequential);
	// the expensive per-file work (hashing) is parallelized later.
	for _, root := range normalizedRoots {
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				errs = append(errs, fmt.Sprintf("walk %s: %v", path, walkErr))
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if d.IsDir() {
				if s.isExcludedDir(path) {
					return fs.SkipDir
				}
				atomic.AddInt64(&dirCount, 1)
				return nil
			}
			atomic.AddInt64(&fileCount, 1)
			meta, merr := s.statFile(path)
			if merr != nil {
				errs = append(errs, fmt.Sprintf("stat %s: %v", path, merr))
				return nil
			}
			select {
			case ch <- meta:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
		if walkErr != nil {
			errs = append(errs, fmt.Sprintf("walk root %s: %v", root, walkErr))
		}
	}

	return atomic.LoadInt64(&fileCount), atomic.LoadInt64(&dirCount), errs, nil
}

// statFile builds FileMeta for a file path.
func (s *Scanner) statFile(path string) (models.FileMeta, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return models.FileMeta{}, err
	}
	return models.FileMeta{
		Path:      path,
		Size:      info.Size(),
		ModTime:   info.ModTime(),
		AccessTime: info.ModTime(),
		IsDir:     info.IsDir(),
		Extension: strings.ToLower(filepath.Ext(path)),
	}, nil
}

// isExcludedDir checks whether the directory path matches the exclude list.
// Pattern may contain '*' wildcard segments.
func (s *Scanner) isExcludedDir(path string) bool {
	if s.excludes == nil {
		return false
	}
	norm := filepath.Clean(os.ExpandEnv(path))
	lowerNorm := strings.ToLower(norm)

	for _, ex := range s.excludes {
		pattern := filepath.Clean(os.ExpandEnv(ex))
		lowerPattern := strings.ToLower(pattern)
		if matchPath(lowerNorm, lowerPattern) {
			return true
		}
		// children of an excluded dir are excluded too
		if strings.HasPrefix(lowerNorm, lowerPattern+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// matchPath matches a cleaned path against a pattern that may contain '*' segments.
func matchPath(path, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return path == pattern
	}
	// Build a simple prefix-based matcher for segment wildcards like
	// C:\Users\*\AppData\Local\Temp
	parts := strings.Split(pattern, string(filepath.Separator))
	idx := 0
	for _, p := range parts {
		if p == "*" {
			if idx == 0 {
				idx = 1
			}
			continue
		}
		// find next matching segment
		tokens := strings.Split(path, string(filepath.Separator))
		found := false
		for ; idx < len(tokens); idx++ {
			if tokens[idx] == p {
				idx++
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}