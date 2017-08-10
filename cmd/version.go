package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version = "1.3.2"
	githash = "HEAD"
)

func init() {
	RootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("haproxy-exporter %s (%s)\n", version, githash)
	},
}
