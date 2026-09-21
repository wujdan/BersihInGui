package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"storage-optimizer/internal/app"
	"storage-optimizer/internal/models"
)

// scanFlags carries scan command options.
type scanFlags struct {
	mode        string
	dryRun      bool
	autoApprove bool
	includeExternal bool
	path        string
}

func newScanCmd() *cobra.Command {
	f := &scanFlags{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Menjalankan pemindaian penyimpanan",
		Long: `Menjalankan pemindaian seluruh drive/partisi aktif dan menampilkan
dashboard interaktif untuk memilih file yang akan dibersihkan.

Mode:
  full   pindai seluruh drive/partisi aktif (default)
  quick  pindai folder umum (Downloads, Temp, Cache)
  custom pindai path spesifik (gunakan flag --path)

Contoh:
  storage-optimizer scan --mode=full
  storage-optimizer scan --mode=full --dry-run
  storage-optimizer scan --mode=custom --path=D:/Project
  storage-optimizer scan --mode=quick --yes`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			roots, err := resolveRoots(f.mode, f.path, f.includeExternal)
			if err != nil {
				return err
			}
			if len(roots) == 0 {
				return fmt.Errorf("tidak ada drive/path yang bisa dipindai")
			}

			if !f.autoApprove {
				fmt.Println("SISTEM PEMBERSIHAN PENYIMPANAN")
				fmt.Println("  Cakupan scan:")
				for _, r := range roots {
					fmt.Printf("    - %s\n", r)
				}
				fmt.Println()
			}

			opts := app.ScanOptions{
				Roots:       roots,
				DryRun:      f.dryRun,
				AutoApprove: f.autoApprove,
			}
			report, err := application.Scan(ctx, opts)
			if err != nil {
				return err
			}

			printScanSummary(report)
			return nil
		},
	}

	cmd.Flags().StringVar(&f.mode, "mode", "full", "mode scan: full, quick, custom")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "simulasi scan tanpa opsi hapus")
	cmd.Flags().BoolVarP(&f.autoApprove, "yes", "y", false, "mode non-interaktif (auto-approve, untuk cron job)")
	cmd.Flags().BoolVar(&f.includeExternal, "include-external", false, "ikutkan drive eksternal/network")
	cmd.Flags().StringVar(&f.path, "path", "", "path spesifik untuk mode custom")
	return cmd
}

// resolveRoots maps mode+flags to scan roots.
func resolveRoots(mode, path string, includeExternal bool) ([]string, error) {
	switch mode {
	case "full":
		return LocalDrives(!includeExternal), nil
	case "quick":
		return quickRoots(), nil
	case "custom":
		if path == "" {
			return nil, fmt.Errorf("mode custom membutuhkan flag --path")
		}
		return []string{path}, nil
	default:
		return nil, fmt.Errorf("mode tidak dikenal: %s (pilihan: full, quick, custom)", mode)
	}
}

// quickRoots returns hot-spot folders (document tahap 1 quick mode).
func quickRoots() []string {
	home, _ := os.UserHomeDir()
	var roots []string
	seen := make(map[string]bool)
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		if _, err := os.Stat(p); err == nil {
			roots = append(roots, p)
			seen[p] = true
		}
	}
	add(home + `\Downloads`)
	add(home + `\AppData\Local\Temp`)
	add(home + `\AppData\Local\Microsoft\Windows\Explorer`)
	add(home + `\AppData\Local\Microsoft\Windows\INetCache`)
	add(os.TempDir())
	return roots
}

// printScanSummary prints a simple report (--yes / non-interactive).
func printScanSummary(r *models.ScanReport) {
	fmt.Println()
	fmt.Println("  HASIL PEMINDAIAN")
	fmt.Println("  --------------------------------------------------------------------------")
	fmt.Printf("  File dipindai : %d\n", r.FilesScanned)
	fmt.Printf("  Direktori     : %d\n", r.DirsScanned)
	fmt.Printf("  Total ukuran  : %s\n", models.HumanBytes(r.TotalBytes))
	fmt.Printf("  Kandidat hapus: %d file (%s)\n", len(r.Candidates), models.HumanBytes(candidatesBytes(r)))
	fmt.Printf("  Waktu         : %s\n", r.Duration.Round(1e6))
	fmt.Println("  --------------------------------------------------------------------------")
	if len(r.Errors) > 0 {
		fmt.Printf("  ℹ %d item dilewati (permission/locked). Lihat log untuk detail.\n", len(r.Errors))
	}
	if len(r.Candidates) == 0 {
		fmt.Println("  ✔ Tidak ada file yang perlu dibersihkan.")
	}
}

func candidatesBytes(r *models.ScanReport) int64 {
	var total int64
	for _, c := range r.Candidates {
		total += c.Meta.Size
	}
	return total
}