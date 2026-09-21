package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"storage-optimizer/internal/config"
)

func newConfigCmd() *cobra.Command {
	gen := false
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Menampilkan atau membuat file konfigurasi",
		Long: `Menampilkan konfigurasi aktif, atau membuat file konfigurasi default.

Contoh:
  storage-optimizer config               # tampilkan konfigurasi aktif
  storage-optimizer config --generate    # buat config/default.yaml`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if gen {
				return generateConfig()
			}
			printConfig()
			return nil
		},
	}
	cmd.Flags().BoolVar(&gen, "generate", false, "buat file config/default.yaml (default konfigurasi)")
	return cmd
}

func generateConfig() error {
	data, err := os.ReadFile("config/default.yaml")
	if err == nil {
		fmt.Println("  ℹ config/default.yaml sudah ada. Membuat salinan di config/default.new.yaml")
		return os.WriteFile("config/default.new.yaml", data, 0o644)
	}
	d := config.Default()
	return d.Save("config/default.yaml")
}

func printConfig() {
	cfg := config.Default()
	_ = cfg
	fmt.Println("  KONFIGURASI AKTIF")
	fmt.Println("  --------------------------------------------------------------------------")
	if cfgPath != "" {
		fmt.Printf("  File konfigurasi : %s\n", cfgPath)
	} else {
		fmt.Println("  File konfigurasi : (default bawaan)")
	}
	fmt.Printf("  Data dir         : %s\n", application.DataDir())
	fmt.Printf("  Retensi default  : %d hari\n", application.Cfg().App.RetentionDays)
	fmt.Println("  --------------------------------------------------------------------------")
	fmt.Println("  Retensi per kategori:")
	fmt.Printf("    Cache & Temp    : %d hari\n", application.Cfg().Retention.CacheTemp)
	fmt.Printf("    Log & Duplikat  : %d hari\n", application.Cfg().Retention.LogDup)
	fmt.Printf("    Installer/Lain  : %d hari\n", application.Cfg().Retention.LargeOld)
	fmt.Println("  --------------------------------------------------------------------------")
	fmt.Println("  Thresholds:")
	fmt.Printf("    File penting (di bawah umur ini tidak ditawarkan untuk hapus): %d hari\n", application.Cfg().Thresholds.ImportantAgeDays)
	fmt.Printf("    File besar (≥ ukuran ini masuk kategori besar): %s\n", human(application.Cfg().Thresholds.LargeFileBytes))
	fmt.Printf("    File lama (tidak diakses ≥ hari ini): %d hari\n", application.Cfg().Thresholds.OldFileDays)
}

func human(b int64) string {
	v := float64(b)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}