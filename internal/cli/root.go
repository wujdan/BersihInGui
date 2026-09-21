package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"storage-optimizer/internal/app"
	"storage-optimizer/internal/config"
)

var (
	cfgPath     string
	cfg         *config.Config
	application *app.App
)

// Name of the binary.
const Name = "storage-optimizer"

// NewRootCmd builds the root command hierarchy.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     Name,
		Short:   "Optimasi penyimpanan laptop/komputer (CLI)",
		Long: `Storage Optimizer memindai drive, mengidentifikasi file yang berpotensi
tidak penting (cache, temp, log, duplikat, installer bekas, file besar lama),
lalu memberi kontrol penuh kepada user untuk memilih dan menyetujui file
yang akan dipindahkan ke karantina sebelum dihapus permanen.

Seluruh proses tercatat dalam audit trail dan file bisa dipulihkan selama
masa retensi.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			c, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			cfg = c
			a, err := app.New(cfg)
			if err != nil {
				return err
			}
			application = a
			return nil
		},
	}

	root.PersistentFlags().StringVar(&cfgPath, "config", "", "path ke file konfigurasi YAML (default: config/default.yaml)")

	root.AddCommand(
		newScanCmd(),
		newReviewCmd(),
		newRestoreCmd(),
		newPurgeCmd(),
		newConfigCmd(),
	)

	// graceful exit context
	return root
}

// Execute runs the CLI and returns the exit code.
func Execute() int {
	root := NewRootCmd()
	root.SilenceUsage = true
	root.SilenceErrors = true

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "✘", err)
		return categorizeExit(err)
	}
	return 0
}

// categorizeExit maps errors to documented exit codes.
func categorizeExit(err error) int {
	switch {
	case err == nil:
		return 0
	default:
		return 1
	}
}