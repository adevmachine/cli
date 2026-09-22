package commands

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/spf13/cobra"
)

func newTunnelCmd(opts *options) *cobra.Command {
	var localPort int

	c := &cobra.Command{
		Use:   "tunnel <workspace> <port>",
		Short: "Reach a port privately, without putting it on the internet",
		Long: "Opens an SSH tunnel so a remote port appears as " +
			"localhost:<port> here. Nothing is published: no DNS record, no " +
			"certificate, no Caddy, and the encryption is SSH's own.\n\n" +
			"It holds the terminal while the tunnel is open. Closing it " +
			"(Ctrl-C) closes the tunnel — there is no state to clean up " +
			"afterwards and nothing to forget about.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspace := args[0]
			remotePort, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("%q is not a port number", args[1])
			}
			local := localPort
			if local == 0 {
				local = remotePort
			}

			tgt, err := workspaceTarget(opts, workspace)
			if err != nil {
				return err
			}
			address, err := firstAddress(tgt.machine)
			if err != nil {
				return err
			}
			if _, err := lookPath("ssh"); err != nil {
				return errors.New("ssh is not installed on this computer")
			}

			// A bind error nobody reads is the worst way to find out a port
			// is already in use; naming it and offering --local is the point.
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", local))
			if err != nil {
				return fmt.Errorf(
					"127.0.0.1:%d is already in use on this computer; pick another with --local: %w", local, err)
			}
			if err := ln.Close(); err != nil {
				return err
			}

			m, user := tgt.machine, tgt.login()
			argv := []string{"-p", strconv.Itoa(m.Port)}
			if m.Key != "" {
				argv = append(argv, "-i", m.Key, "-o", "IdentitiesOnly=yes")
			}
			argv = append(argv, "-N", "-L", fmt.Sprintf("%d:127.0.0.1:%d", local, remotePort), user+"@"+address)

			cmd.Printf("tunnel open: localhost:%d -> %s:%d on %s. Ctrl-C to close.\n",
				local, workspace, remotePort, tgt.machine.Name)
			return runInteractive("ssh", argv...)
		},
	}
	c.Flags().IntVar(&localPort, "local", 0, "the port on this computer (default: the same as the remote port)")
	return c
}
