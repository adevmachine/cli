package commands

import (
	"fmt"

	"github.com/adevmachine/cli/internal/remote"
	"github.com/adevmachine/cli/internal/stats"
	"github.com/spf13/cobra"
)

func newStatsCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Report what the machine is spending",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			client, address, err := remote.Dial(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer client.Close()

			s, err := stats.Collect(cmd.Context(), client)
			if err != nil {
				return err
			}

			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), s)
			}

			cmd.Printf("host    %s\n", address)
			cmd.Printf("cpus    %d, load %.2f %.2f %.2f\n", s.CPUs, s.Load1, s.Load5, s.Load15)
			cmd.Printf("memory  %s used of %s, %s available\n",
				humanBytes(s.MemUsedBytes), humanBytes(s.MemTotalBytes), humanBytes(s.MemAvailableBytes))
			if s.SwapTotalBytes > 0 {
				cmd.Printf("swap    %s used of %s\n", humanBytes(s.SwapUsedBytes), humanBytes(s.SwapTotalBytes))
			} else {
				cmd.Printf("swap    none\n")
			}
			cmd.Printf("disk    %s used of %s, %s free\n",
				humanBytes(s.DiskUsedBytes), humanBytes(s.DiskTotalBytes), humanBytes(s.DiskAvailBytes))
			return nil
		},
	}
}

// humanBytes renders a byte count for a person. The JSON output keeps the raw
// number, so nothing downstream has to parse this back.
func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	value, exp := float64(n)/unit, 0
	for value >= unit && exp < 4 {
		value /= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", value, "KMGTP"[exp])
}
