package classifier

import (
	"path/filepath"
	"strings"
	"time"

	"storage-optimizer/internal/config"
	"storage-optimizer/internal/models"
	"storage-optimizer/internal/orphan"
)

// importantExtensions are files never offered for deletion.
var importantExtensions = map[string]bool{
	// source code / projects
	".go": true, ".rs": true, ".py": true, ".js": true, ".ts": true, ".jsx": true,
	".tsx": true, ".java": true, ".c": true, ".cpp": true, ".h": true, ".hpp": true,
	".cs": true, ".rb": true, ".php": true, ".sql": true, ".sh": true, ".bat": true,
	".ps1": true, ".vue": true, ".swift": true, ".kt": true, ".toml": true,
	".json": true, ".yaml": true, ".yml": true, ".xml": true, ".config": true,
	// documents
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
	".pdf": true, ".txt": true, ".odt": true, ".rtf": true, ".md": true, ".markdown": true,
	".epub": true, ".mobi": true,
	// media (personal)
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true, ".tiff": true,
	".svg": true, ".webp": true, ".raw": true, ".cr2": true, ".nef": true, ".heic": true,
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".wmv": true, ".flv": true,
	".webm": true, ".m4v": true, ".mp3": true, ".wav": true, ".flac": true, ".aac": true,
	".m4a": true, ".ogg": true, ".opus": true,
	// personal data
	".vcf": true, ".ics": true, ".pst": true, ".ost": true, ".eml": true,
	// databases / project data
	".db": true, ".sqlite": true, ".sqlite3": true, ".mdb": true, ".accdb": true,
	".dwg": true, ".dxf": true, ".psd": true, ".ai": true,
	// keepsakes: hidden / dotfiles
	".gitignore": true, ".gitkeep": true, ".env": true,
}

// cacheExtensions recognized temporary/cache file types.
var cacheExtensions = map[string]bool{
	".tmp": true, ".temp": true, ".cache": true, ".dmp": true, ".log_old": true,
	".bak_tmp": true, ".swp": true, ".swo": true, ".lock": true,
	".icns": true, ".pnx": true, ".eth": true,
	// windows
	".evo": true, ".blf": true, ".wpre": true, ".edi": true,
	// update caches
	".cab_tmp": true,
}

// installerExtensions trigger installer classification.
var installerExtensions = map[string]bool{
	".msi": true, ".msix": true, ".dmg": true, ".pkg": true, ".deb": true,
	".rpm": true, ".iso": false, // iso also checked separately
	".jar_tmp": true,
}

// importantCacheNames are well-known cache filenames to treat as safe.
var cacheDirNames = map[string]bool{
	"cache": true, "cacheddata": true, "temp": true, "tmp": true,
	"__pycache__": true, ".cache": true,
}

// logExtensions used in the log classifier.
var logExtensions = map[string]bool{
	".log": true, ".logs": true, ".logt": true,
}

// Classifier labels files as candidates for deletion using
// rule-based heuristics with confidence scores.
type Classifier struct {
	cfg     *config.Config
	orphans *orphan.Detector
}

// New creates a classifier bound to a config.
func New(cfg *config.Config) *Classifier {
	d := orphan.New()
	d.Load()
	return &Classifier{cfg: cfg, orphans: d}
}

// Classify evaluates a file and returns a Candidate if it is removable,
// or nil if the file is considered important/safe to keep.
func (c *Classifier) Classify(f *models.FileMeta, dupGroup string) *models.Candidate {
	now := time.Now()
	if f.IsDir {
		return nil
	}

	ext := strings.ToLower(f.Extension)
	base := strings.ToLower(filepath.Base(f.Path))

	// 1. Important files are NEVER candidates (document tahap 5).
	if importantExtensions[ext] || importantExtensions[base] {
		return nil
	}

	// 2. Cache & temp (high confidence). Age is irrelevant here: being in a
	//    cache/temp location is the signal. Recently touched files that are
	//    actually in use will be caught by the silent validator (lock check).
	if c.isCacheOrTemp(f, ext, base) {
		return &models.Candidate{
			Meta:       f,
			Label:      models.LabelSafe,
			Category:   models.CategoryCache,
			Confidence: 0.96,
			Reason:     "File cache/temporary: " + c.reasonCache(f, base),
		}
	}

	ageDays := now.Sub(f.ModTime).Hours() / 24

	// 3. Old log files (high confidence).
	if logExtensions[ext] && ageDays >= float64(c.cfg.Thresholds.LogAgeDays) {
		return &models.Candidate{
			Meta:       f,
			Label:      models.LabelSafe,
			Category:   models.CategoryLog,
			Confidence: 0.94,
			Reason:     "File log yang sudah berumur",
		}
	}

	// 4. Old installers (medium confidence).
	if installerExtensions[ext] && ageDays >= float64(c.cfg.Thresholds.InstallerAgeDays) {
		return &models.Candidate{
			Meta:       f,
			Label:      models.LabelReview,
			Category:   models.CategoryInstaller,
			Confidence: 0.68,
			Reason:     "Installer lama, sudah berumur",
		}
	}

	// 5. Duplicates (medium-high confidence).
	if dupGroup != "" {
		return &models.Candidate{
			Meta:           f,
			Label:          models.LabelReview,
			Category:       models.CategoryDuplicate,
			Confidence:     0.88,
			DuplicateGroup: dupGroup,
			Reason:         "Duplikat terdeteksi (hash sama)",
		}
	}

	// 6. Leftover data from uninstalled apps (orphan). Confidence stays
	// moderate because install-state detection is heuristic; labeling review
	// lets the user verify before removal.
	if c.orphans != nil && c.orphans.Configured() {
		if o := c.orphans.IsOrphan(f.Path); o != nil {
			return &models.Candidate{
				Meta:       f,
				Label:      models.LabelReview,
				Category:   models.CategoryOrphan,
				Confidence: 0.62,
				Reason:     "Data sisa aplikasi yang sudah dihapus: " + o.AppName,
			}
		}
	}

	// 7. Recently modified files are assumed actively used → safe.
	if ageDays <= float64(c.cfg.Thresholds.ImportantAgeDays) {
		return nil
	}

	// 8. Large files not accessed for a long time (lower confidence).
	if f.Size >= c.cfg.Thresholds.LargeFileBytes {
		return &models.Candidate{
			Meta:       f,
			Label:      models.LabelReview,
			Category:   models.CategoryLargeOld,
			Confidence: 0.55,
			Reason:     "File besar tidak diakses lama",
		}
	}

	return nil
}

// isCacheOrTemp implements cache/temp detection rules.
func (c *Classifier) isCacheOrTemp(f *models.FileMeta, ext, base string) bool {
	// extension-based
	if cacheExtensions[ext] {
		return true
	}
	// name based common cache files
	if strings.HasPrefix(base, ".~") || strings.HasSuffix(base, "~") ||
		strings.HasPrefix(base, ".nfs") || base == "thumbs.db" {
		return true
	}
	// directory-based: path contains a cache/temp dir
	dirs := strings.ToLower(strings.ReplaceAll(filepath.Dir(f.Path), "\\", "/"))
	if strings.Contains(dirs, "/cache") ||
		strings.Contains(dirs, "/temp") ||
		strings.Contains(dirs, "/tmp") ||
		strings.Contains(dirs, "/trash") ||
		strings.Contains(dirs, "/preview") {
		return true
	}
	// browser caches
	if strings.Contains(dirs, "appdata/local/microsoft/windows/webcache") ||
		strings.Contains(dirs, "appdata/local/google/chrome/user data/default/cache") ||
		strings.Contains(dirs, "appdata/local/microsoft/edge/user data/default/cache") {
		return true
	}
	// dotnet/msbuild temp cache patterns
	if strings.HasPrefix(base, "tmp") && cacheExtensions[".tmp"] {
		return true
	}
	return false
}

// reasonCache produces a short human readable explanation.
func (c *Classifier) reasonCache(f *models.FileMeta, base string) string {
	switch {
	case strings.HasPrefix(base, ".~") || strings.HasSuffix(base, "~"):
		return "temp editor backup"
	case base == "thumbs.db":
		return "thumbnail cache windows"
	case strings.Contains(strings.ToLower(f.Dir()), "cache"):
		return "folder cache"
	default:
		return "extension " + f.Extension
	}
}