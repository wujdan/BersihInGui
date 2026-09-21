package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"storage-optimizer/internal/store"
	"storage-optimizer/internal/ui"
)

func newReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Membuka dashboard hasil scan terakhir",
		Long: `Membuka dashboard interaktif dari hasil scan terakhir yang tersimpan.
User dapat meninjau, menyesuaikan pilihan, dan menyetujui pembersihan.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			report, err := store.LoadLatestReport(application.DataDir())
			if err != nil {
				return err
			}
			moveFn := func(sel map[string]bool) ui.MoveOutcome {
				return application.MoveSelected(ctx, report, sel)
			}
			res, err := ui.Run(report, moveFn)
			if err != nil {
				return err
			}
			if res.Approved {
				printReviewDone()
			}
			return nil
		},
	}
	return cmd
}

func printReviewDone() {
	fmt.Println()
	fmt.Println("  ✔ Pembersihan selesai. Gunakan: storage-optimizer restore | purge.")
}