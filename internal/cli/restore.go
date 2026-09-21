package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"storage-optimizer/internal/models"
	"storage-optimizer/internal/quarantine"
)

func newRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore [file-id | all]",
		Short: "Memulihkan file dari quarantine",
		Long: `Memulihkan file yang dipindahkan ke quarantine kembali ke lokasi aslinya.

Contoh:
  storage-optimizer restore              # daftar file yang bisa dipulihkan
  storage-optimizer restore <file-id>    # pulihkan satu file
  storage-optimizer restore all          # pulihkan semua file`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := application.Quarantine()

			if len(args) == 0 {
				printQuarantineList(q.List())
				return nil
			}

			switch args[0] {
			case "all":
				ms := q.List()
				if len(ms) == 0 {
					fmt.Println("  ℹ Quarantine kosong.")
					return nil
				}
				var restored int
				for _, m := range ms {
					if _, err := q.Restore(m.ID); err != nil {
						fmt.Printf("  ✘ %s (%s): %v\n", m.ID, m.OriginalPath, err)
						continue
					}
					fmt.Printf("  ✔ %s -> %s\n", m.ID, m.OriginalPath)
					restored++
				}
				application.Logger().Info("restore", "restore semua", map[string]interface{}{"count": restored})
				fmt.Printf("\n  Pulihkan %d file.\n", restored)
				return nil
			default:
				m, err := q.Restore(args[0])
				if err != nil {
					return err
				}
				application.Logger().Info("restore", "restore file", map[string]interface{}{"id": m.ID, "path": m.OriginalPath})
				fmt.Printf("  ✔ Dipulihkan: %s\n    -> %s\n", m.ID, m.OriginalPath)
				return nil
			}
		},
	}
	return cmd
}

// printQuarantineList shows quarantined files (restore screen).
func printQuarantineList(ms []*quarantine.Manifest) {
	if len(ms) == 0 {
		fmt.Println("  ℹ Quarantine kosong — tidak ada file untuk dipulihkan.")
		return
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].MovedAt.After(ms[j].MovedAt) })
	fmt.Println("  FILE DALAM QUARANTINE")
	fmt.Println("  -----------------------------------------------------------------------------------------------")
	fmt.Printf("  %-22s %-10s %14s %-14s %s\n", "ID", "Kategori", "Ukuran", "Retensi s.d.", "Lokasi asli")
	fmt.Println("  -----------------------------------------------------------------------------------------------")
	for _, m := range ms {
		fmt.Printf("  %-22s %-10s %14s %-14s %s\n",
			m.ID, m.Category, models.HumanBytes(m.Size),
			m.RetentionUntil.Format("2006-01-02"), m.OriginalPath)
	}
	fmt.Println("  -----------------------------------------------------------------------------------------------")
	fmt.Println("  Gunakan: storage-optimizer restore <file-id>  untuk memulihkan.")
}