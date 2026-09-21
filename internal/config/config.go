package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Default directory nam cluster for the app.
const AppName = "storage-optimizer"

// Config holds all runtime configuration for the application.
type Config struct {
	App      AppConfig      `yaml:"app"`
	Scan     ScanConfig     `yaml:"scan"`
	Exclude  ExcludeConfig  `yaml:"exclude"`
	Retention RetentionConfig `yaml:"retention"`
	Thresholds Thresholds   `yaml:"thresholds"`
}

type AppConfig struct {
	DataDir       string `yaml:"data_dir"`
	RetentionDays int    `yaml:"retention_days"`
}

type ScanConfig struct {
	IncludeExternal     bool `yaml:"include_external"`
	WorkerCount         int  `yaml:"worker_count"` // 0 = auto
	HashThresholdBytes  int64 `yaml:"hash_threshold_bytes"`
	DedupMinSize        int64 `yaml:"dedup_min_size"`
}

type ExcludeConfig struct {
	Windows        []string `yaml:"windows"`
	Linux          []string `yaml:"linux"`
	Darwin         []string `yaml:"darwin"`
	Custom         []string `yaml:"custom"`
}

type RetentionConfig struct {
	CacheTemp int `yaml:"cache_temp_days"`
	LogDup    int `yaml:"log_duplicate_days"`
	LargeOld  int `yaml:"large_old_days"`
}

type Thresholds struct {
	ImportantAgeDays int  `yaml:"important_age_days"`
	LargeFileBytes   int64 `yaml:"large_file_bytes"`
	OldFileDays      int  `yaml:"old_file_days"`
	InstallerAgeDays int  `yaml:"installer_age_days"`
	LogAgeDays       int  `yaml:"log_age_days"`
}

// Default returns a Config populated with platform-aware defaults.
func Default() *Config {
	cfg := &Config{
		App: AppConfig{
			DataDir:       defaultDataDir(),
			RetentionDays: 30,
		},
		Scan: ScanConfig{
			IncludeExternal:     false,
			WorkerCount:         0,
			HashThresholdBytes:  100 * 1024 * 1024, // 100 MB
			DedupMinSize:        1024 * 1024,       // 1 MB
		},
		Exclude: ExcludeConfig{
			Windows: []string{
				`C:\Windows`,
				`C:\Program Files`,
				`C:\Program Files (x86)`,
				`C:\ProgramData`,
				`C:\$Recycle.Bin`,
				`C:\Windows.old`,
				`C:\System Volume Information`,
				`C:\Users\*\AppData\Local\Microsoft\Windows\INetCache`,
				`C:\Users\*\AppData\Local\Microsoft\Windows\Explorer`,
				`C:\Users\*\AppData\Local\Packages\Microsoft.Windows.Search_cw5n1h2txyewy`,
				`C:\Users\*\AppData\Local\Microsoft\Windows\Caches`,
				`C:\Users\*\AppData\Local\Microsoft\Windows\TxR`,
				`C:\Users\*\AppData\Local\Temp`,
				`C:\Users\*\AppData\Local\CrashDumps`,
			},
			Linux: []string{
				"/proc", "/sys", "/dev", "/boot", "/run",
				"/snap", "/var/lib/docker", "/var/cache/apt",
			},
			Darwin: []string{
				"/System", "/Library", "/var/db", "/private/var/vm",
			},
			Custom: []string{},
		},
		Retention: RetentionConfig{
			CacheTemp: 14,
			LogDup:    30,
			LargeOld:  90,
		},
		Thresholds: Thresholds{
			ImportantAgeDays: 30,
			LargeFileBytes:   100 * 1024 * 1024, // 100 MB
			OldFileDays:      365,
			InstallerAgeDays: 90,
			LogAgeDays:       90,
		},
	}
	return cfg
}

// Load reads config from a YAML file, merging into defaults.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	return cfg, nil
}

// Save writes the config to path.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Normalize ensures paths are cleaned and data dir exists.
func (c *Config) Normalize() error {
	if c.App.DataDir == "" {
		c.App.DataDir = defaultDataDir()
	}
	if dir := os.ExpandEnv(c.App.DataDir); dir != "" {
		c.App.DataDir = dir
	}
	if err := os.MkdirAll(c.App.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	return nil
}

// PlatformExcludes returns effective exclude list for current platform + custom.
func (c *Config) PlatformExcludes() []string {
	var list []string
	switch runtime.GOOS {
	case "windows":
		list = append(list, c.Exclude.Windows...)
	case "linux":
		list = append(list, c.Exclude.Linux...)
	case "darwin":
		list = append(list, c.Exclude.Darwin...)
	}
	list = append(list, c.Exclude.Custom...)
	return list
}

// ImportFormatValues helper: converts a bool-ish CLI string.
func ParseBool(s string) (bool, error) {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, fmt.Errorf("invalid bool %q", s)
	}
	return b, nil
}

// dataDirs returns important application directories names.
func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".storage-optimizer")
	}
	return filepath.Join(home, ".storage-optimizer")
}

// ExpandDataDir expands env vars in a data-dir style path.
func ExpandDataDir(dir string) string {
	expanded := os.ExpandEnv(dir)
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(expanded, "~") {
		if home != "" {
			expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~"))
		}
	}
	abs, err := filepath.Abs(expanded)
	if err == nil {
		return abs
	}
	return expanded
}