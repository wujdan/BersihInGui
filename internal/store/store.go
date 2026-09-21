package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"storage-optimizer/internal/models"
)

const reportFile = "last-scan-report.json"

// SaveReport persists a ScanReport to the data directory.
func SaveReport(dataDir string, report *models.ScanReport) (string, error) {
	dir := filepath.Join(dataDir, "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Join(dir, fmt.Sprintf("scan-%s.json", report.GeneratedAt.Format("20060102-150405")))
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(name, data, 0o644); err != nil {
		return "", err
	}
	// also keep the latest as a stable "last" report
	latest := filepath.Join(dir, reportFile)
	_ = os.WriteFile(latest, data, 0o644)
	return name, nil
}

// LoadLatestReport returns the most recent scan report.
func LoadLatestReport(dataDir string) (*models.ScanReport, error) {
	latest := filepath.Join(dataDir, "reports", reportFile)
	data, err := os.ReadFile(latest)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("belum ada hasil scan tersimpan. Jalankan: storage-optimizer scan --mode=full")
		}
		return nil, err
	}
	var r models.ScanReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse laporan terakhir: %w", err)
	}
	return &r, nil
}

// LoadReport reads a report from an explicit path.
func LoadReport(path string) (*models.ScanReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r models.ScanReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// TouchUpdated writes an "updated" note for the capacity display.
func TouchUpdated(dataDir string, freed int64) error {
	dir := filepath.Join(dataDir, "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	rec := struct {
		UpdatedAt time.Time `json:"updated_at"`
		Freed     int64     `json:"freed_bytes"`
	}{time.Now(), freed}
	data, _ := json.Marshal(rec)
	return os.WriteFile(filepath.Join(dir, "last-cleanup.json"), data, 0o644)
}