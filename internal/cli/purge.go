package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"storage-optimizer/internal/models"
)

func newPurgeCmd() *cobra.Command {
	now := false
	force := false
	cmd := &cobra.Command{
		Use:   "purge [file-id...]",
		Short: "Menghapus permanen isi quarantine",
		Long: `Menghapus permanen file yang sudah melewati masa retensi di quarantine.
File dalam masa retensi TIDAK akan dihapus kecuali dengan flag --force-days.

Contoh:
  storage-optimizer purge               # hapus permanen file yang sudah melewati retensi
  storage-optimizer purge --now         # hapus permanen semua file segera
  storage-optimizer purge <file-id>     # hapus permanen satu file tertentu`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := application.Quarantine()

			ids := args
			if now {
				ids = nil // purge everything
			}

			count, freed, err := q.PurgeExpired(ids, now)
			if err != nil {
				return err
			}
			application.Logger().Info("purge", "purge quarantine", map[string]interface{}{
				"count": count, "freed": freed, "forced": now,
			})
			if count == 0 {
				fmt.Println("  ℹ Tidak ada file yang memenuhi syarat untuk dihapus permanen.")
				fmt.Println("    Gunakan --now untuk menghapus semua isi quarantine segera.")
				return nil
			}
			fmt.Printf("  ✔ %d file dihapus permanen (%s)\n", count, models.HumanBytes(freed))
			return nil
		},
	}
	cmd.Flags().BoolVar(&now, "now", false, "hapus permanen SEMUA isi quarantine segera")
	cmd.Flags().BoolVar(&force, "force-days", false, "abaikan masa retensi (tidak dipakai; gunakan --now)")
	return cmd
}