package cmd

import (
	"errors"

	"github.com/pix-platform/aha/cmd/check"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "aha",
		Short: "aha 是一个小工具",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("no additional command provided")
		},
	}

	rootCmd.AddCommand(
		check.New(),
	)

	return rootCmd
}
