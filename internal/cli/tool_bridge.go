package cli

import (
	"os"

	"github.com/shhac/lib-agent-harness/session"
	"github.com/spf13/cobra"
)

// toolBridge is how a worker's coding CLI reaches the daemon's tools. The CLI
// starts this process itself; it relays that CLI's tool protocol to the
// assignment that configured it and does nothing else.
//
// It is deliberately inert. It holds no policy, executes no tool, interprets
// nothing it carries and takes no arguments: the private channel, its
// credential and its lock are named by the environment the session set for this
// process, and a bridge started any other way refuses to run. Running it by
// hand does nothing.
func toolBridge() *cobra.Command {
	return &cobra.Command{
		Use:    "tool-bridge",
		Short:  "Relay a worker coding session's tool protocol to its assignment",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return session.RunBridge(cmd.Context(), os.Stdin, os.Stdout)
		},
	}
}
