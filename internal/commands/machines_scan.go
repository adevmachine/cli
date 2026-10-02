package commands

import (
	"errors"

	"github.com/mydevmachine/devmachine/internal/config"
	"github.com/mydevmachine/devmachine/internal/hostkeys"
	"github.com/spf13/cobra"
)

// scanResult is the host key an address presented, before anything trusts it.
type scanResult struct {
	Address     string `json:"address"`
	Port        int    `json:"port"`
	KeyType     string `json:"key_type"`
	Fingerprint string `json:"fingerprint"`
	// Verify prints the same key's fingerprint on the server itself, to run
	// from its console rather than through this connection.
	Verify string `json:"verify,omitempty"`
}

func newMachinesScanCmd(opts *options) *cobra.Command {
	var (
		address string
		port    int
	)
	c := &cobra.Command{
		Use:   "scan",
		Short: "Show the SSH host key an address presents, trusting and writing nothing",
		Long: "Reads the host key a server presents, without logging in, for a " +
			"machine that is not configured yet. Nothing is trusted and nothing is " +
			"written: it is what to compare against the provider console before " +
			"`machines add --fingerprint` trusts it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if address == "" {
				return errors.New("scan needs --address: the server to read the host key from")
			}
			m := config.Machine{Name: address, Hosts: []config.Host{{Address: address}}, Port: port}
			if dir, _, err := config.Dir(opts.configDir); err == nil {
				m.ConfigDir = dir
				if cfg, found, err := loadConfigIfAny(opts); err == nil && found {
					m.PackagesRelease = cfg.Packages
				}
			}
			presented, answered, err := scanHostKey(cmd.Context(), m)
			if err != nil {
				return err
			}
			result := scanResult{
				Address:     answered,
				Port:        port,
				KeyType:     presented.Type(),
				Fingerprint: hostkeys.Fingerprint(presented),
				Verify:      hostKeyVerifyCommand(presented.Type()),
			}
			if opts.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), result)
			}
			cmd.Printf("%s port %d presented %s %s\n", result.Address, result.Port, result.KeyType, result.Fingerprint)
			if result.Verify != "" {
				cmd.Printf("verify it from the server's own console: %s\n", result.Verify)
			}
			cmd.Printf("once it matches: devmachine machines add --address %s --port %d --fingerprint %s …\n",
				address, port, result.Fingerprint)
			cmd.Println("Nothing was trusted or written.")
			return nil
		},
	}
	c.Flags().StringVar(&address, "address", "", "the server's address: an IP or a hostname")
	c.Flags().IntVar(&port, "port", config.DefaultPort, "the SSH port")
	return c
}
