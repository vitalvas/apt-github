package signing

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gpgAvailable() bool {
	_, err := exec.LookPath("gpg")
	return err == nil
}

func TestSetupAndClearSign(t *testing.T) {
	if !gpgAvailable() {
		t.Skip("gpg not available")
	}

	tmpDir := t.TempDir()
	gpgHome := filepath.Join(tmpDir, "gpg")
	pubKeyPath := filepath.Join(tmpDir, "keyrings", "apt-transport-github.gpg")

	err := Setup(gpgHome, pubKeyPath)
	require.NoError(t, err)

	assert.FileExists(t, pubKeyPath)

	signer := NewGPGSigner(gpgHome)

	content := []byte("Origin: github\nLabel: test/repo\nSuite: stable\n")

	signed, err := signer.ClearSign(content)
	require.NoError(t, err)

	signedStr := string(signed)
	assert.Contains(t, signedStr, "-----BEGIN PGP SIGNED MESSAGE-----")
	assert.Contains(t, signedStr, "Origin: github")
	assert.Contains(t, signedStr, "-----BEGIN PGP SIGNATURE-----")
	assert.Contains(t, signedStr, "-----END PGP SIGNATURE-----")
}

func TestClearSignVerify(t *testing.T) {
	if !gpgAvailable() {
		t.Skip("gpg not available")
	}

	tmpDir := t.TempDir()
	gpgHome := filepath.Join(tmpDir, "gpg")
	pubKeyPath := filepath.Join(tmpDir, "keyrings", "apt-transport-github.gpg")

	require.NoError(t, Setup(gpgHome, pubKeyPath))

	signer := NewGPGSigner(gpgHome)

	content := []byte("Package: test\nVersion: 1.0.0\n")

	signed, err := signer.ClearSign(content)
	require.NoError(t, err)

	signedFile := filepath.Join(tmpDir, "signed.txt")
	require.NoError(t, os.WriteFile(signedFile, signed, 0644))

	verifyCmd := exec.Command("gpg",
		"--homedir", gpgHome,
		"--verify", signedFile,
	)

	out, err := verifyCmd.CombinedOutput()
	require.NoError(t, err, "verification failed: %s", string(out))
	assert.True(t, strings.Contains(string(out), "Good signature"))
}

func TestClearSignWithoutKey(t *testing.T) {
	if !gpgAvailable() {
		t.Skip("gpg not available")
	}

	tmpDir := t.TempDir()
	gpgHome := filepath.Join(tmpDir, "empty-gpg")
	require.NoError(t, os.MkdirAll(gpgHome, 0700))

	signer := NewGPGSigner(gpgHome)

	content := []byte("test content")
	_, err := signer.ClearSign(content)
	assert.Error(t, err)
}

func TestSetupCreatesDirectories(t *testing.T) {
	if !gpgAvailable() {
		t.Skip("gpg not available")
	}

	tmpDir := t.TempDir()
	gpgHome := filepath.Join(tmpDir, "deep", "nested", "gpg")
	pubKeyPath := filepath.Join(tmpDir, "deep", "keyrings", "apt-transport-github.gpg")

	err := Setup(gpgHome, pubKeyPath)
	require.NoError(t, err)

	assert.DirExists(t, gpgHome)
	assert.FileExists(t, pubKeyPath)

	info, err := os.Stat(pubKeyPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestEnsureShortHomedir(t *testing.T) {
	t.Run("short path unchanged", func(t *testing.T) {
		shortPath := "/tmp/gpg"
		result, cleanup, err := ensureShortHomedir(shortPath)

		require.NoError(t, err)
		assert.Nil(t, cleanup)
		assert.Equal(t, shortPath, result)
	})

	t.Run("long path gets symlink", func(t *testing.T) {
		tmpDir := t.TempDir()
		longPath := filepath.Join(tmpDir, "a-very-long-directory-name-that-exceeds-the-socket-path-limit")
		require.NoError(t, os.MkdirAll(longPath, 0700))

		result, cleanup, err := ensureShortHomedir(longPath)
		require.NoError(t, err)
		require.NotNil(t, cleanup)

		defer cleanup()

		assert.NotEqual(t, longPath, result)
		assert.True(t, strings.HasPrefix(result, "/tmp/apt-transport-github-gpg-"))

		target, err := os.Readlink(result)
		require.NoError(t, err)
		assert.Equal(t, longPath, target)
	})
}

func TestNewGPGSigner(t *testing.T) {
	signer := NewGPGSigner("/test/path")
	assert.Equal(t, "/test/path", signer.HomeDir)
}

func TestSetupInvalidGPGHome(t *testing.T) {
	if !gpgAvailable() {
		t.Skip("gpg not available")
	}

	err := Setup("/dev/null/invalid", "/tmp/test-key.gpg")
	assert.Error(t, err)
}

func TestSetupInvalidPubKeyPath(t *testing.T) {
	if !gpgAvailable() {
		t.Skip("gpg not available")
	}

	tmpDir := t.TempDir()
	gpgHome := filepath.Join(tmpDir, "gpg")

	err := Setup(gpgHome, "/dev/null/invalid/key.gpg")
	assert.Error(t, err)
}
