package setup

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gpgAvailable() bool {
	_, err := exec.LookPath("gpg")
	return err == nil
}

func TestRunRequiresRoot(t *testing.T) {
	var buf bytes.Buffer

	err := Run(&buf, 1000, "/tmp/gpg", "/tmp/key.gpg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "root")
}

func TestRunSuccess(t *testing.T) {
	if !gpgAvailable() {
		t.Skip("gpg not available")
	}

	tmpDir := t.TempDir()
	gpgHome := filepath.Join(tmpDir, "gpg")
	pubKeyPath := filepath.Join(tmpDir, "keyrings", "apt-transport-github.gpg")

	var buf bytes.Buffer

	err := Run(&buf, 0, gpgHome, pubKeyPath)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Setup complete.")
	assert.Contains(t, output, pubKeyPath)
	assert.Contains(t, output, "signed-by=")
}
