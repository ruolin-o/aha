package main

import (
	"github.com/pix-platform/aha/cmd"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := cmd.New()
	cobra.CheckErr(rootCmd.Execute())
}
