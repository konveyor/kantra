package provider

import (
	"fmt"
	"io"

	"github.com/konveyor-ecosystem/kantra/pkg/util"
	"github.com/spf13/cobra"
)

var (
	containerProviders = []string{
		util.JavaProvider,
		util.PythonProvider,
		util.GoProvider,
		util.CsharpProvider,
		util.NodeJSProvider,
	}
	containerlessProviders = []string{
		util.JavaProvider,
	}
)

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available analysis providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ListProviders(cmd.OutOrStdout())
		},
	}
}

// ListProviders writes supported analysis providers to w.
func ListProviders(w io.Writer) error {
	fmt.Fprintln(w, "container analysis supported providers:")
	for _, prov := range containerProviders {
		if _, err := fmt.Fprintln(w, prov); err != nil {
			return err
		}
	}
	fmt.Fprintln(w, "containerless analysis supported providers (default):")
	for _, prov := range containerlessProviders {
		if _, err := fmt.Fprintln(w, prov); err != nil {
			return err
		}
	}
	return nil
}
