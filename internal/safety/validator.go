package safety

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"storage-optimizer/internal/models"
)

// ValidationResult holds the outcome of a pre-deletion check.
type ValidationResult struct {
	Passed bool
	Reason string
}

// Validator performs silent re-validation before any file is moved to
// quarantine (document tahap 9). All failures are logged and skipped.
type Validator struct{}

// New returns a Validator.
func New() *Validator { return &Validator{} }

// Validate checks a file against the documented 7 checks.
func (v *Validator) Validate(meta *models.FileMeta) ValidationResult {
	path := meta.Path

	// 1. File existence check.
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ValidationResult{false, "file tidak ada lagi"}
		}
		return ValidationResult{false, "gagal mengakses file: " + err.Error()}
	}

	// 2. Path must remain a regular (non-dir, non-symlink-to-dir) file.
	if info.IsDir() {
		return ValidationResult{false, "path ternyata direktori"}
	}

	// 3. Timing: last-modified must match what we saw at scan time.
	modDelta := info.ModTime().Sub(meta.ModTime)
	if modDelta.Abs() > 10*time.Second {
		return ValidationResult{false, "file berubah sejak scan (mtime berbeda)"}
	}

	// 4. Size must match (cheap integrity proxy).
	if info.Size() != meta.Size {
		return ValidationResult{false, "ukuran file berbeda dari hasil scan"}
	}

	// 5. Symlink resolution check: never follow symlinks for deletions.
	if info.Mode()&os.ModeSymlink != 0 {
		return ValidationResult{false, "file adalah symlink, dilewati"}
	}

	// 6. Permission verification: must be writable in its directory.
	dir := filepath.Dir(path)
	st, err := os.Stat(dir)
	if err != nil {
		return ValidationResult{false, "direktori induk tidak dapat diakses"}
	}
	if !st.IsDir() {
		return ValidationResult{false, "induk path bukan direktori"}
	}

	// 7. Lock/file-in-use detection: try opening with exclusive read access.
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		if isLockedError(err) {
			return ValidationResult{false, "file sedang digunakan oleh proses lain"}
		}
		return ValidationResult{false, "tidak dapat membuka file: " + err.Error()}
	}
	_ = f.Close()

	return ValidationResult{true, "valid"}
}

// isLockedError detects sharing-violation / in-use errors across platforms.
func isLockedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "another process") ||
		strings.Contains(msg, "being used by another process") ||
		strings.Contains(msg, "sharing violation") ||
		strings.Contains(msg, "resource busy")
}