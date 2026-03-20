package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCmdRunsMethod(t *testing.T) {
	var stdout bytes.Buffer

	cmd := NewRootCmdWithIO(strings.NewReader(""), &stdout)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "100 Capabilities")
}

func TestNewRootCmd(t *testing.T) {
	cmd := NewRootCmd()

	assert.Equal(t, "apt-github", cmd.Use)
	assert.NotNil(t, cmd.RunE)
}

func TestSetupSubcommand(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"setup"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "root")
}

func TestUnknownSubcommand(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"unknown"})

	err := cmd.Execute()
	assert.Error(t, err)
}

func TestSetupCmdExists(t *testing.T) {
	cmd := NewRootCmd()

	setupCmd, _, err := cmd.Find([]string{"setup"})
	require.NoError(t, err)
	assert.Equal(t, "setup", setupCmd.Use)
}
