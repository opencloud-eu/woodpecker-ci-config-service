package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wcs",
	Short: "a swiss army knife to manage woodpecker ci configuration files",
}
