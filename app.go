package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"storage-optimizer/internal/classifier"
	"storage-optimizer/internal/config"
	"storage-optimizer/internal/logging"
	"storage-optimizer/internal/models"
	"storage-optimizer/internal/quarantine"
	"storage-optimizer/internal/safety"
	"storage-optimizer/internal/scanner"
	"storage-optimizer/internal/store"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-backed application struct.
type App struct {
	ctx    context.Context
	cfg    *config.Config
	logger *logging.Logger
	q      *quarantine.Dir

	mu       sync.Mutex
	lastReport *models.ScanReport
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	cfg, err := config.Load("")
	if err != nil {
		return
	}
	if err := cfg.Normalize(); err != nil {
		return
	}
	a.cfg = cfg
	if err := os.MkdirAll(filepath.Join(cfg.App.DataDir, "logs"), 0o755); err != nil {
		return
	}
	logger, err := logging.New(filepath.Join(cfg.App.DataDir, "logs"))
	if err != nil {
		return
	}
	a.logger = logger
	q, err := quarantine.New(cfg)
	if err != nil {
		return
	}
	a.q = q
}

// Shutdown closes resources.
func (a *App) Shutdown(ctx context.Context) {
	if a.logger != nil {
		_ = a.logger.Close()
	}
}

// DriveInfo describes a drive/scan root for the UI.
type DriveInfo struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	Removable bool   `json:"removable"`
	FreeBytes int64  `json:"free_bytes"`
	TotalBytes int64 `json:"total_bytes"`
}

// GetDrives returns local drives available for scanning.
func (a *App) GetDrives() []DriveInfo {
	var out []DriveInfo
	for _, root := range rootCandidates() {
		dt, removable := detectDriveType(root)
		free, total := diskSpace(root)
		out = append(out, DriveInfo{Path: root, Type: dt, Removable: removable, FreeBytes: free, TotalBytes: total})
	}
	if len(out) == 0 {
		out = append(out, DriveInfo{Path: string(filepath.Separator), Type: "local", Removable: false})
	}
	return out
}

// AppInfo returns basic app information for the settings page.
func (a *App) AppInfo() map[string]interface{} {
	dataDir := ""
	if a.cfg != nil {
		dataDir = a.cfg.App.DataDir
	}
	return map[string]interface{}{
		"version":      "1.0.0",
		"data_dir":     dataDir,
		"platform":     runtime.GOOS,
	}
}

// BrowseFolder opens a native directory picker so the user can scan any
// folder on the system, not just drive roots.
func (a *App) BrowseFolder() string {
	if a.ctx == nil {
		return ""
	}
	dir, err := wailsrt.OpenDirectoryDialog(a.ctx, wailsrt.OpenDialogOptions{
		Title:                "Pilih Folder untuk Dipindai",
		CanCreateDirectories: false,
	})
	if err != nil || dir == "" {
		return ""
	}
	return dir
}

// CategoryInfo aggregates candidates per category for the UI.
type CategoryInfo struct {
	Category   string           `json:"category"`
	Count      int              `json:"count"`
	TotalBytes int64            `json:"total_bytes"`
	TotalHuman string           `json:"total_human"`
	Confidence float64          `json:"confidence"`
	Files      []CandidateInfo  `json:"files"`
}

// CandidateInfo is a serializable candidate.
type CandidateInfo struct {
	Key        string  `json:"key"`
	Path       string  `json:"path"`
	Name       string  `json:"name"`
	Size       int64   `json:"size"`
	SizeHuman  string  `json:"size_human"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	Label      string  `json:"label"`
	Modified   string  `json:"modified"`
	Type       string  `json:"type"`
}

// ScanProgress carries live progress updates to the UI.
type ScanProgress struct {
	Files       int64  `json:"files"`
	Dirs        int64  `json:"dirs"`
	Phase       string `json:"phase"`
	CurrentPath string `json:"current_path"`
	Done        bool   `json:"done"`
	Root        string `json:"root"`
}

// ScanReportDTO is what the UI receives after a scan.
type ScanReportDTO struct {
	GeneratedAt  string         `json:"generated_at"`
	DurationMs   float64        `json:"duration_ms"`
	Roots        []string       `json:"roots"`
	FilesScanned int            `json:"files_scanned"`
	DirsScanned  int            `json:"dirs_scanned"`
	TotalBytes   int64          `json:"total_bytes"`
	TotalHuman   string         `json:"total_human"`
	Candidates   []CategoryInfo `json:"candidates"`
	Errors       []string       `json:"errors"`
}

// ScanResult is the full output type.
type ScanResult struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	Report  ScanReportDTO `json:"report"`
}

// MoveProgress carries live per-file progress while moving to quarantine.
type MoveProgress struct {
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Path    string `json:"path"`
	Name    string `json:"name"`
	Status  string `json:"status"` // moving | done | skip | fail
	Message string `json:"message,omitempty"`
	Done    bool   `json:"done"`
}

// MoveOutcome is the result of quarantining selected files.
type MoveOutcome struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message"`
	Succeeded int      `json:"succeeded"`
	Failed    int      `json:"failed"`
	Freed     int64    `json:"freed"`
	FreedHuman string   `json:"freed_human"`
	Details   []string `json:"details"`
	MovedKeys []string `json:"moved_keys"`
}

// QuarantineItem is a serializable quarantine entry.
type QuarantineItem struct {
	ID           string `json:"id"`
	OriginalPath string `json:"original_path"`
	QuarantinePath string `json:"quarantine_path"`
	Size         int64  `json:"size"`
	SizeHuman    string `json:"size_human"`
	Category     string `json:"category"`
	Label        string `json:"label"`
	MovedAt      string `json:"moved_at"`
	RetentionUntil string `json:"retention_until"`
	Expired      bool   `json:"expired"`
}

// Scan starts a scan of the given root and returns the report DTO.
func (a *App) Scan(root string, mode string) ScanResult {
	if a.cfg == nil {
		return ScanResult{Success: false, Message: "konfigurasi belum siap"}
	}
	roots, err := resolveScanRoot(root, mode)
	if err != nil {
		return ScanResult{Success: false, Message: err.Error()}
	}

	// Emit a "scan:started" event
	wailsrt.EventsEmit(a.ctx, "scan:progress", ScanProgress{Phase: "start", Root: root, Done: false})

	start := time.Now()
	a.logger.Info(logging.EventScanStarted, "pemindaian dimulai", map[string]interface{}{
		"roots": roots, "mode": mode,
	})

	s := scanner.New(a.cfg, roots)
	filesCh := make(chan models.FileMeta, 256)

	report := &models.ScanReport{GeneratedAt: time.Now(), Roots: roots, Candidates: make([]*models.Candidate, 0)}

	type outcome struct {
		count, d int64
		errs     []string
	}
	outcomeCh := make(chan outcome, 1)
	go func() {
		count, d, errs, _ := s.Scan(a.ctx, filesCh)
		outcomeCh <- outcome{count, d, errs}
		close(filesCh)
	}()

	// Single consumer loop: collect files for classification AND emit
	// live progress events to the UI (throttled to every 150ms).
	var (
		files      []models.FileMeta
		lastEmit   = time.Now()
	)
	for f := range filesCh {
		files = append(files, f)
		if now := time.Now(); now.Sub(lastEmit) > 150*time.Millisecond {
			lastEmit = now
			wailsrt.EventsEmit(a.ctx, "scan:progress", ScanProgress{
				Files: int64(len(files)), Dirs: 0, Phase: "scan", CurrentPath: f.Path,
			})
		}
	}

	res := <-outcomeCh
	report.FilesScanned = int(res.count)
	report.DirsScanned = int(res.d)
	report.Errors = res.errs
	report.TotalBytes = 0
	for _, f := range files {
		report.TotalBytes += f.Size
	}

	// deduplication + classification
	wailsrt.EventsEmit(a.ctx, "scan:progress", ScanProgress{Phase: "dedup", Done: false})
	dupMap := scanner.DetectDuplicates(a.ctx, a.cfg, files, 0)
	cl := classifier.New(a.cfg)
	for i := range files {
		f := &files[i]
		if c := cl.Classify(f, dupMap[f.Path]); c != nil {
			report.Candidates = append(report.Candidates, c)
		}
	}
	report.Duration = time.Since(start)

	if _, err := store.SaveReport(a.cfg.App.DataDir, report); err != nil {
		return ScanResult{Success: false, Message: "gagal simpan laporan: " + err.Error()}
	}

	a.mu.Lock()
	a.lastReport = report
	a.mu.Unlock()

	dto := buildReportDTO(report)

	wailsrt.EventsEmit(a.ctx, "scan:progress", ScanProgress{
		Phase: "done", Files: int64(report.FilesScanned), Dirs: int64(report.DirsScanned),
		Done: true,
	})
	return ScanResult{Success: true, Report: dto}
}

// resolveScanRoot interprets UI selection.
func resolveScanRoot(root, mode string) ([]string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, os.ErrInvalid
	}
	switch mode {
	case "full":
		return scannerRoots(root), nil
	default:
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			return nil, os.ErrInvalid
		}
		return []string{root}, nil
	}
}

// scannerRoots returns local drive roots for a "full" scan.
func scannerRoots(_ string) []string {
	var out []string
	for _, r := range rootCandidates() {
		if _, removable := detectDriveType(r); removable {
			continue
		}
		out = append(out, r)
	}
	return out
}

// GetLastReport returns the last stored scan report (if any).
func (a *App) GetLastReport() (ScanReportDTO, error) {
	if a.cfg == nil {
		return ScanReportDTO{}, os.ErrInvalid
	}
	r, err := store.LoadLatestReport(a.cfg.App.DataDir)
	if err != nil {
		return ScanReportDTO{}, err
	}
	return buildReportDTO(r), nil
}

// MoveToQuarantine moves selected candidate files into quarantine.
func (a *App) MoveToQuarantine(keys []string) MoveOutcome {
	if a.q == nil {
		return MoveOutcome{Success: false, Message: "quarantine belum siap"}
	}
	a.mu.Lock()
	report := a.lastReport
	a.mu.Unlock()
	if report == nil {
		return MoveOutcome{Success: false, Message: "belum ada hasil scan"}
	}

	// validate each selected candidate
	validator := safety.New()
	var succeeded, failed int
	var freed int64
	var details []string
	var movedKeys []string

	// build candidate lookup once (avoids O(n²) scan per key)
	byKey := make(map[string]*models.Candidate, len(report.Candidates))
	for _, c := range report.Candidates {
		byKey[c.Key()] = c
	}

	// prepare quarantine blob dir once for the whole batch (no per-file MkdirAll)
	blobDir, err := a.q.BlobDirFor(time.Now().Format("2006-01-02"))
	if err != nil {
		return MoveOutcome{Success: false, Message: "karantina belum siap"}
	}
	var seq int64

	var (
		mu       sync.Mutex
		emitMu   sync.Mutex
		lastEmit time.Time
	)
	sem := make(chan struct{}, 16)
	processed := int64(0)
	emit := func(current int64, key, status, message string) {
		emitMu.Lock()
		// throttle: at most ~10 events/s; always let the first and final pass
		if !lastEmit.IsZero() && time.Since(lastEmit) < 100*time.Millisecond && current < int64(len(keys)) {
			emitMu.Unlock()
			return
		}
		lastEmit = time.Now()
		emitMu.Unlock()
		a.emitMoveProgress(int(current), len(keys), key, status, message)
	}

	// Phase 1 (parallel): SHA-256 each file concurrently. Renames must stay
	// serial on NTFS (~10x slower in parallel), but hashing scales with
	// workers, so split the two stages.
	var stagedMu sync.Mutex
	staged := make([]*quarantine.Staged, 0, len(keys))
	var stageWg sync.WaitGroup
	for _, key := range keys {
		cand := byKey[key]
		if cand == nil {
			failed++
			details = append(details, "tidak ditemukan: "+key)
			emit(atomic.AddInt64(&processed, 1), key, "skip", "tidak ditemukan")
			continue
		}
		v := validator.Validate(cand.Meta)
		if !v.Passed {
			failed++
			details = append(details, "skip "+filepath.Base(key)+": "+v.Reason)
			emit(atomic.AddInt64(&processed, 1), key, "skip", v.Reason)
			continue
		}

		sem <- struct{}{}
		stageWg.Add(1)
		go func(cand *models.Candidate, key string) {
			defer stageWg.Done()
			defer func() { <-sem }()
			s, err := a.q.Stage(cand, blobDir, &seq)
			if err != nil {
				mu.Lock()
				failed++
				details = append(details, "gagal "+filepath.Base(key)+": "+err.Error())
				mu.Unlock()
				emit(atomic.LoadInt64(&processed), key, "fail", err.Error())
				return
			}
			stagedMu.Lock()
			staged = append(staged, s)
			stagedMu.Unlock()
		}(cand, key)
		atomic.AddInt64(&processed, 1)
	}
	stageWg.Wait()

	// Phase 2 (serial): rename into quarantine + record manifest. Serial is
	// faster than any parallel schedule here on NTFS.
	for i, s := range staged {
		key := s.Candidate().Key()
		if _, err := a.q.Commit(s); err != nil {
			mu.Lock()
			failed++
			details = append(details, "gagal "+filepath.Base(key)+": "+err.Error())
			mu.Unlock()
			emit(atomic.LoadInt64(&processed), key, "fail", err.Error())
			continue
		}
		mu.Lock()
		succeeded++
		freed += s.Candidate().Meta.Size
		movedKeys = append(movedKeys, key)
		details = append(details, "✔ "+filepath.Base(key))
		mu.Unlock()
		emit(atomic.LoadInt64(&processed), key, "done", "")

		// two middle checkpoints so a crash mid-batch does not lose much
		// progress; avoids O(n²) by not persisting every few thousand.
		done := i + 1
		if done == len(staged)/3 || done == len(staged)*2/3 {
			if perr := a.q.Persist(); perr != nil {
				mu.Lock()
				details = append(details, "peringatan: persist "+perr.Error())
				mu.Unlock()
			}
		}
	}
	if perr := a.q.Persist(); perr != nil {
		mu.Lock()
		details = append(details, "peringatan: persist "+perr.Error())
		mu.Unlock()
	}
	a.emitMoveProgress(len(keys), len(keys), "", "done", "")

	return MoveOutcome{
		Success:    succeeded > 0,
		Message:    "Berhasil memindahkan " + itoa(succeeded) + " file",
		Succeeded:  succeeded,
		Failed:     failed,
		Freed:      freed,
		FreedHuman: models.HumanBytes(freed),
		Details:    details,
		MovedKeys:  movedKeys,
	}
}

// emitMoveProgress sends a live quarantine progress update to the frontend.
func (a *App) emitMoveProgress(current, total int, path, status, message string) {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, "quarantine:progress", &MoveProgress{
		Current: current,
		Total:   total,
		Path:    path,
		Name:    filepath.Base(path),
		Status:  status,
		Message: message,
		Done:    current >= total && (status == "done" || status == "fail"),
	})
}

// Restore restores selected quarantined files (empty list = restore all).
func (a *App) Restore(ids []string) MoveOutcome {
	if a.q == nil {
		return MoveOutcome{Success: false, Message: "quarantine belum siap"}
	}
	manifests := a.q.List()
	selected := make(map[string]bool)
	for _, id := range ids {
		selected[id] = true
	}
	freed := int64(0)
	count := 0
	failed := []string{}
	for _, m := range manifests {
		if len(selected) > 0 && !selected[m.ID] {
			continue
		}
		if _, err := a.q.Restore(m.ID); err != nil {
			failed = append(failed, filepath.Base(m.OriginalPath)+": "+err.Error())
			continue
		}
		count++
		freed += m.Size
	}
	return MoveOutcome{
		Success: len(failed) == 0, Message: itoa(count) + " file dipulihkan",
		Succeeded: count, Freed: freed, FreedHuman: models.HumanBytes(freed),
		Details: failed,
	}
}

// RestoreSingle restores a single quarantined file by ID.
func (a *App) RestoreSingle(id string) MoveOutcome {
	return a.Restore([]string{id})
}

// PurgeSingle purges a single quarantined file by ID.
func (a *App) PurgeSingle(id string) MoveOutcome {
	return a.PurgeSelected([]string{id})
}

// PurgeSelected permanently deletes the given quarantined files
// immediately (no waiting for retention to expire).
func (a *App) PurgeSelected(ids []string) MoveOutcome {
	if a.q == nil {
		return MoveOutcome{Success: false, Message: "quarantine belum siap"}
	}
	count, freed, err := a.q.PurgeExpired(ids, true)
	if err != nil {
		return MoveOutcome{Success: false, Message: err.Error()}
	}
	return MoveOutcome{
		Success: count > 0, Message: itoa(count) + " file dihapus permanen",
		Succeeded: count, Freed: freed, FreedHuman: models.HumanBytes(freed),
	}
}

// Purge purges quarantined files that expired (or all with force).
func (a *App) Purge(force bool) MoveOutcome {
	if a.q == nil {
		return MoveOutcome{Success: false, Message: "quarantine belum siap"}
	}
	count, freed, err := a.q.PurgeExpired(nil, force)
	if err != nil {
		return MoveOutcome{Success: false, Message: err.Error()}
	}
	return MoveOutcome{
		Success: true, Message: itoa(count) + " file dibersihkan",
		Succeeded: count, Freed: freed, FreedHuman: models.HumanBytes(freed),
	}
}

// ListQuarantine returns quarantine items for the UI.
func (a *App) ListQuarantine() []QuarantineItem {
	if a.q == nil {
		return nil
	}
	now := time.Now()
	var out []QuarantineItem
	for _, m := range a.q.List() {
		out = append(out, QuarantineItem{
			ID:             m.ID,
			OriginalPath:   m.OriginalPath,
			QuarantinePath: m.QuarantinePath,
			Size:           m.Size,
			SizeHuman:      models.HumanBytes(m.Size),
			Category:       m.Category,
			Label:          m.Label,
			MovedAt:        m.MovedAt.Local().Format("2006-01-02 15:04"),
			RetentionUntil: m.RetentionUntil.Local().Format("2006-01-02"),
			Expired:        now.After(m.RetentionUntil),
		})
	}
	return out
}

// Stats returns overall quota information.
func (a *App) Stats() map[string]interface{} {
	qItems := a.ListQuarantine()
	var totalBytes int64
	for _, m := range qItems {
		totalBytes += m.Size
	}
	return map[string]interface{}{
		"quarantine_count": len(qItems),
		"quarantine_bytes": totalBytes,
	}
}

func buildReportDTO(r *models.ScanReport) ScanReportDTO {
	dto := ScanReportDTO{
		GeneratedAt:  r.GeneratedAt.Format("2006-01-02 15:04:05"),
		DurationMs:   r.Duration.Seconds() * 1000,
		Roots:        r.Roots,
		FilesScanned: r.FilesScanned,
		DirsScanned:  r.DirsScanned,
		TotalBytes:   r.TotalBytes,
		TotalHuman:   models.HumanBytes(r.TotalBytes),
		Errors:       r.Errors,
		Candidates:   []CategoryInfo{},
	}
	for _, cs := range r.ByCategory() {
		cat := CategoryInfo{
			Category:   string(cs.Category),
			Count:      cs.Count,
			TotalBytes: cs.TotalBytes,
			TotalHuman: models.HumanBytes(cs.TotalBytes),
			Confidence: cs.Confidence,
			Files:      []CandidateInfo{},
		}
		for _, c := range cs.Files {
			if c.Meta == nil {
				continue
			}
			cat.Files = append(cat.Files, CandidateInfo{
				Key:        c.Key(),
				Path:       c.Meta.Path,
				Name:       c.Meta.Name(),
				Size:       c.Meta.Size,
				SizeHuman:  models.HumanBytes(c.Meta.Size),
				Category:   string(c.Category),
				Confidence: c.Confidence,
				Reason:     c.Reason,
				Label:      string(c.Label),
				Modified:   c.Meta.ModTime.Format("2006-01-02"),
				Type:       c.Meta.Extension,
			})
		}
		dto.Candidates = append(dto.Candidates, cat)
	}
	return dto
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}