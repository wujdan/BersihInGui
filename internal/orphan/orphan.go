// Package orphan detects leftover application data folders whose owning
// application is no longer installed (orphaned app-data).
package orphan

import (
	"os"
	"path/filepath"
	"strings"
)

// knownSafeFolderNames are app-data folder segments that belong to the OS
// or shared components and must never be flagged as orphaned.
var knownSafeFolderNames = map[string]bool{
	"microsoft": true, "windows": true, "packages": true, "programs": true,
	"temp": true, "crashdumps": true, "elevateddiagnostics": true,
	"intel": true, "nvidia": true, "amd": true, "realtek": true,
	"common": true, "roaming": true, "local": true, "locallow": true,
	"fonts": true, "help": true, "internet explorer": true,
	"shellnew": true, "start menu": true, "desktop": true, "favorites": true,
	"history": true, "templates": true, "cookies": true,
}

// Result describes a detected orphaned app-data folder.
type Result struct {
	AppName string // readable app segment, e.g. "Google\Chrome"
}

// Detector identifies app-data folders whose application is not installed.
type Detector struct {
	installed map[string]bool // normalized words gathered from install data
	roots     []string        // app-data roots (Local, Roaming, LocalLow, ProgramData)
	loaded    bool
}

// New creates an empty detector.
func New() *Detector {
	return &Detector{installed: map[string]bool{}}
}

// Load builds the installed-application index. Platform specific backends
// (registry on Windows) are wired in orphan_windows.go / orphan_other.go.
func (d *Detector) Load() {
	if d.loaded {
		return
	}
	d.loadInstalled()
	d.roots = appDataRoots()
	d.loaded = true
}

// IsOrphan reports whether path lives inside an app-data root whose owning
// application is no longer installed. Returns nil when the app is present,
// the folder is known-safe, or detection data is unavailable.
func (d *Detector) IsOrphan(path string) *Result {
	if !d.loaded {
		return nil
	}
	clean := filepath.Clean(path)
	lower := strings.ToLower(clean)
	for _, root := range d.roots {
		rl := strings.ToLower(filepath.Clean(root))
		if !strings.HasPrefix(lower, rl+string(filepath.Separator)) {
			continue
		}
		rel := strings.TrimPrefix(lower, rl+string(filepath.Separator))
		segs := strings.Split(rel, string(filepath.Separator))
		if len(segs) == 0 {
			continue
		}
		// Company\Product layouts (e.g. Google\Chrome): if either of the first
		// two segments belongs to an installed app, the data is NOT orphaned.
		checked := 0
		for i := 0; i < len(segs) && i < 2; i++ {
			name := sed_word(segs[i])
			if name == "" {
				continue
			}
			checked++
			if knownSafeFolderNames[name] {
				return nil
			}
			if d.isInstalled(name) {
				return nil
			}
		}
		if checked == 0 {
			continue
		}
		return &Result{AppName: sed_org(clean, root)}
	}
	return nil
}

// Configured reports whether detection is usable on the current platform.
func (d *Detector) Configured() bool {
	return d.loaded && len(d.installed) > 0
}

func (d *Detector) isInstalled(word string) bool {
	if d.installed[word] {
		return true
	}
	// fall back to word-level tokens ("7-zip" -> "zip")
	for _, tok := range normalizeWords(word) {
		if d.installed[tok] {
			return true
		}
	}
	return false
}

func (d *Detector) addInstalledName(name string) {
	for _, w := range normalizeWords(name) {
		d.installed[w] = true
	}
}

// addInstalledPath indexes the meaningful segments of an install location
// so that app-data segments like "Program Files\Google\Chrome" match.
func (d *Detector) addInstalledPath(p string) {
	path := strings.ToLower(filepath.Clean(p))
	segs := strings.FieldsFunc(path, func(r rune) bool {
		return r == '\\' || r == '/'
	})
	for _, s := range segs {
		for _, w := range normalizeWords(s) {
			d.installed[w] = true
		}
	}
}

// normalizeWords converts a string into lowercase alphanumeric word tokens.
func normalizeWords(s string) []string {
	s = strings.ToLower(s)
	replaced := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return ' '
	}, s)
	var out []string
	for _, f := range strings.Fields(replaced) {
		if len(f) >= 3 {
			out = append(out, f)
		}
	}
	return out
}

// sed_word lowercases a path segment for lookup.
func sed_word(seg string) string {
	return strings.ToLower(strings.TrimSpace(seg))
}

// sed_org returns the first path segment (original case) below the root.
func sed_org(path, root string) string {
	base := strings.TrimPrefix(path, root)
	base = strings.TrimLeft(base, "\\/")
	segs := strings.FieldsFunc(base, func(r rune) bool { return r == '\\' || r == '/' })
	if len(segs) == 0 {
		return base
	}
	if len(segs) >= 2 && !knownSafeFolderNames[sed_word(segs[0])] {
		return segs[0] + `\` + segs[1]
	}
	return segs[0]
}

// appDataRoots returns the well-known application-data roots present on disk.
func appDataRoots() []string {
	var roots []string
	for _, env := range []string{"LOCALAPPDATA", "APPDATA"} {
		if v := os.Getenv(env); v != "" {
			roots = append(roots, v)
		}
	}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		roots = append(roots, filepath.Join(filepath.Dir(la), "LocalLow"))
	}
	if pd := os.Getenv("ProgramData"); pd != "" {
		roots = append(roots, pd)
	}
	return roots
}