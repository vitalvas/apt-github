package setup

import (
	"fmt"
	"io"

	"github.com/vitalvas/apt-transport-github/internal/signing"
)

func Run(w io.Writer, euid int, gpgHome, pubKeyPath string) error {
	if euid != 0 {
		return fmt.Errorf("setup must be run as root")
	}

	fmt.Fprintln(w, "Generating GPG signing key...")

	if err := signing.Setup(gpgHome, pubKeyPath); err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}

	fmt.Fprintln(w, "Setup complete.")
	fmt.Fprintf(w, "Public key exported to: %s\n", pubKeyPath)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Add repositories with:")
	fmt.Fprintf(w, "  deb [signed-by=%s] github://OWNER/REPO stable main\n", pubKeyPath)

	return nil
}
