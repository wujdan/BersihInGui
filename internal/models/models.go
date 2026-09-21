package models

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Label indicates how safe a file is to remove.
type Label string

const (
	LabelImportant Label = "PENTING"
	LabelSafe      Label = "AMAN"
	LabelReview    Label = "PERLU_REVIEW"
)

// Category groups files into human readable buckets.
type Category string

const (
	CategoryCache     Category = "Cache & Temp"
	CategoryLog       Category = "Log Files"
	CategoryInstaller Category = "Installer Lama"
	CategoryLargeOld  Category = "File Besar Tidak Terpakai"
	CategoryDuplicate Category = "Duplikat"
	CategoryOrphan    Category = "Sisa Aplikasi Terhapus"
)

// CategoryColors provides ANSI color names per category (used by UI).
func (c Category) Color() string {
	switch c {
	case CategoryCache:
		return "cyan"
	case CategoryLog:
		return "magenta"
	case CategoryInstaller:
		return "yellow"
	case CategoryLargeOld:
		return "orange/red"
	case CategoryDuplicate:
		return "blue"
	case CategoryOrphan:
		return "purple"
	default:
		return "white"
	}
}

// FileMeta is the metadata captured during scanning.
type FileMeta struct {
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	ModTime    time.Time `json:"mod_time"`
	AccessTime time.Time `json:"access_time"`
	Extension  string    `json:"extension"`
	IsDir      bool      `json:"is_dir"`
}

// Name returns base filename.
func (f *FileMeta) Name() string {
	return filepath.Base(f.Path)
}

// Dir returns the directory containing the file.
func (f *FileMeta) Dir() string {
	return filepath.Dir(f.Path)
}

// Candidate is a file flagged as a removable candidate.
type Candidate struct {
	Meta           *FileMeta `json:"meta"`
	Label          Label     `json:"label"`
	Category       Category  `json:"category"`
	Confidence     float64   `json:"confidence"`
	DuplicateGroup string    `json:"duplicate_group,omitempty"`
	Reason         string    `json:"reason"`
}

// Key returns a stable id for a candidate. Returns empty string when the
// file metadata is unavailable (e.g. loaded from an older report).
func (c *Candidate) Key() string {
	if c == nil || c.Meta == nil {
		return ""
	}
	return c.Meta.Path
}

// CategorySummary aggregates a category for the dashboard.
type CategorySummary struct {
	Category   Category
	Count      int
	TotalBytes int64
	Confidence float64
	Files      []*Candidate
}

// ScanReport is the result of a full scan.
type ScanReport struct {
	GeneratedAt  time.Time          `json:"generated_at"`
	Duration     time.Duration      `json:"duration_seconds"`
	Roots        []string           `json:"roots"`
	FilesScanned int                `json:"files_scanned"`
	TotalBytes   int64              `json:"total_bytes"`
	DirsScanned  int                `json:"dirs_scanned"`
	Candidates   []*Candidate       `json:"candidates"`
	Errors       []string           `json:"errors"`
}

// ByCategory groups candidates into ordered summaries.
func (r *ScanReport) ByCategory() []*CategorySummary {
	order := []Category{
		CategoryCache,
		CategoryLog,
		CategoryInstaller,
		CategoryLargeOld,
		CategoryDuplicate,
		CategoryOrphan,
	}
	sums := make(map[Category]*CategorySummary)
	for _, c := range r.Candidates {
		if c == nil || c.Meta == nil {
			continue
		}
		s := sums[c.Category]
		if s == nil {
			s = &CategorySummary{Category: c.Category}
			sums[c.Category] = s
		}
		s.Count++
		s.TotalBytes += c.Meta.Size
		s.Confidence += c.Confidence
		s.Files = append(s.Files, c)
	}
	var out []*CategorySummary
	for _, cat := range order {
		if s := sums[cat]; s != nil {
			if s.Count > 0 {
				s.Confidence /= float64(s.Count)
			}
			out = append(out, s)
		}
	}
	return out
}

// TotalSelected computes totals for a list of selected candidates.
func (r *ScanReport) TotalSelected(selected map[string]bool) (int, int64) {
	count, size := 0, int64(0)
	for _, c := range r.Candidates {
		if c == nil || c.Meta == nil {
			continue
		}
		if selected[c.Key()] {
			count++
			size += c.Meta.Size
		}
	}
	return count, size
}

// HumanBytes renders a byte count in human readable form.
func HumanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// YesNo converts a bool to the Indonesian yes/no text used in dialogs.
func YesNo(b bool) string {
	if b {
		return "ya"
	}
	return "tidak"
}

// JoinList joins a list with a separator and handles empty list.
func JoinList(items []string, sep string) string {
	filtered := make([]string, 0, len(items))
	for _, i := range items {
		if s := strings.TrimSpace(i); s != "" {
			filtered = append(filtered, s)
		}
	}
	return strings.Join(filtered, sep)
}