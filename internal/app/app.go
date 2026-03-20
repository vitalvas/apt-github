package app

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/vitalvas/apt-github/internal/method"
	"github.com/vitalvas/apt-github/internal/setup"
	"github.com/vitalvas/apt-github/internal/signing"
)

func NewRootCmd() *cobra.Command {
	return NewRootCmdWithIO(os.Stdin, os.Stdout)
}

func NewRootCmdWithIO(stdin io.Reader, stdout io.Writer) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "apt-github",
		Short:        "APT transport method for GitHub releases",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			signer := signing.NewGPGSigner(signing.DefaultGPGHome)
			m := method.NewWithSigner(signer)

			return m.Run(stdin, stdout)
		},
	}

	rootCmd.AddCommand(newSetupCmd())

	return rootCmd
}

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Generate GPG signing key for APT repository metadata",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return setup.Run(cmd.OutOrStdout(), os.Geteuid(), signing.DefaultGPGHome, signing.DefaultPubKey)
		},
	}
}
