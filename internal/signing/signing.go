package signing

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	DefaultGPGHome = "/etc/apt-transport-github/gpg"
	DefaultPubKey  = "/etc/apt/keyrings/apt-transport-github.gpg"
	KeyName        = "apt-transport-github (APT GitHub Transport)"

	maxSocketPathLen = 70
)

type Signer interface {
	ClearSign(content []byte) ([]byte, error)
}

type GPGSigner struct {
	HomeDir string
}

func NewGPGSigner(homeDir string) *GPGSigner {
	return &GPGSigner{
		HomeDir: homeDir,
	}
}

func (s *GPGSigner) ClearSign(content []byte) ([]byte, error) {
	effectiveHome, cleanup, err := ensureShortHomedir(s.HomeDir)
	if err != nil {
		return nil, fmt.Errorf("gpg home setup failed: %w", err)
	}

	if cleanup != nil {
		defer cleanup()
	}

	cmd := exec.Command("gpg",
		"--homedir", effectiveHome,
		"--batch",
		"--pinentry-mode", "loopback",
		"--passphrase", "",
		"--default-key", KeyName,
		"--clearsign",
	)

	cmd.Stdin = bytes.NewReader(content)

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gpg clearsign failed: %s", string(exitErr.Stderr))
		}

		return nil, fmt.Errorf("gpg clearsign failed: %w", err)
	}

	return out, nil
}

func Setup(gpgHome, pubKeyPath string) error {
	if err := os.MkdirAll(gpgHome, 0700); err != nil {
		return fmt.Errorf("failed to create GPG home directory: %w", err)
	}

	effectiveHome, cleanup, err := ensureShortHomedir(gpgHome)
	if err != nil {
		return fmt.Errorf("failed to prepare GPG home: %w", err)
	}

	if cleanup != nil {
		defer cleanup()
	}

	genCmd := exec.Command("gpg",
		"--homedir", effectiveHome,
		"--batch",
		"--pinentry-mode", "loopback",
		"--passphrase", "",
		"--quick-generate-key",
		KeyName,
		"ed25519",
		"sign",
		"0",
	)

	if out, err := genCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to generate GPG key: %s", string(out))
	}

	if err := os.MkdirAll(filepath.Dir(pubKeyPath), 0755); err != nil {
		return fmt.Errorf("failed to create keyrings directory: %w", err)
	}

	exportCmd := exec.Command("gpg",
		"--homedir", effectiveHome,
		"--batch",
		"--export",
		"--output", pubKeyPath,
		KeyName,
	)

	if out, err := exportCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to export public key: %s", string(out))
	}

	return nil
}

func ensureShortHomedir(gpgHome string) (string, func(), error) {
	socketPath := filepath.Join(gpgHome, "S.gpg-agent")
	if len(socketPath) < maxSocketPathLen {
		return gpgHome, nil, nil
	}

	shortDir, err := os.MkdirTemp("/tmp", "apt-transport-github-gpg-")
	if err != nil {
		return "", nil, err
	}

	if err := os.Remove(shortDir); err != nil {
		return "", nil, err
	}

	if err := os.Symlink(gpgHome, shortDir); err != nil {
		return "", nil, err
	}

	cleanup := func() {
		killCmd := exec.Command("gpgconf", "--homedir", shortDir, "--kill", "gpg-agent")
		killCmd.Run()

		os.Remove(shortDir)
	}

	return shortDir, cleanup, nil
}
