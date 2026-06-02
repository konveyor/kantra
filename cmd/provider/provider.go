package provider

import (
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
)

func NewProviderCommand(_ logr.Logger) *cobra.Command {
	providerCmd := &cobra.Command{
		Use:   "provider",
		Short: "Inspect analysis providers",
	}
	providerCmd.AddCommand(newListCommand())
	return providerCmd
}
