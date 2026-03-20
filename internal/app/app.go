package app

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/vitalvas/apt-github/internal/cache"
	"github.com/vitalvas/apt-github/internal/github"
	"github.com/vitalvas/apt-github/internal/method"
	"github.com/vitalvas/apt-github/internal/setup"
	"github.com/vitalvas/apt-github/internal/signing"
)

func NewRootCmd(version string) *cobra.Command {
	return NewRootCmdWithIO(version, os.Stdin, os.Stdout)
}

func NewRootCmdWithIO(version string, stdin io.Reader, stdout io.Writer) *cobra.Command {
	github.SetVersion(version)

	rootCmd := &cobra.Command{
		Use:          "apt-github",
		Short:        "APT transport method for GitHub releases",
		Version:      version,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			signer := signing.NewGPGSigner(signing.DefaultGPGHome)
			m := method.NewWithSigner(signer)

			return m.Run(stdin, stdout)
		},
	}

	rootCmd.AddCommand(newSetupCmd())
	rootCmd.AddCommand(newCleanCmd())

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

func newCleanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove cached release metadata and package control data",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := cache.New(cache.DefaultBaseDir)
			if err := c.Clean(); err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Cache cleaned.")

			return nil
		},
	}
}
