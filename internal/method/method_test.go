package method

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/apt-github/internal/github"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		expected  *parsedURI
		expectErr bool
	}{
		{
			name: "release path",
			uri:  "github://owner/repo/dists/stable/Release",
			expected: &parsedURI{
				Owner:    "owner",
				Repo:     "repo",
				Path:     "dists/stable/Release",
				Versions: defaultVersions,
			},
		},
		{
			name: "packages path",
			uri:  "github://vitalvas/systemd-supervisord/dists/stable/main/binary-amd64/Packages",
			expected: &parsedURI{
				Owner:    "vitalvas",
				Repo:     "systemd-supervisord",
				Path:     "dists/stable/main/binary-amd64/Packages",
				Versions: defaultVersions,
			},
		},
		{
			name: "pool path",
			uri:  "github://owner/repo/pool/v1.0.0/test_1.0.0_amd64.deb",
			expected: &parsedURI{
				Owner:    "owner",
				Repo:     "repo",
				Path:     "pool/v1.0.0/test_1.0.0_amd64.deb",
				Versions: defaultVersions,
			},
		},
		{
			name: "custom versions",
			uri:  "github://owner/repo/dists/stable/Release?versions=20",
			expected: &parsedURI{
				Owner:    "owner",
				Repo:     "repo",
				Path:     "dists/stable/Release",
				Versions: 20,
			},
		},
		{
			name:      "too short",
			uri:       "github://owner",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseURI(tt.uri)
			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, parsed)
		})
	}
}

func TestExtractArch(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{name: "amd64", path: "dists/stable/main/binary-amd64/Packages", expected: "amd64"},
		{name: "arm64", path: "dists/stable/main/binary-arm64/Packages.gz", expected: "arm64"},
		{name: "no arch", path: "dists/stable/Release", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractArch(tt.path))
		})
	}
}

type mockSigner struct{}

func (s *mockSigner) ClearSign(content []byte) ([]byte, error) {
	var buf bytes.Buffer

	fmt.Fprintln(&buf, "-----BEGIN PGP SIGNED MESSAGE-----")
	fmt.Fprintln(&buf, "Hash: SHA256")
	fmt.Fprintln(&buf)
	buf.Write(content)
	fmt.Fprintln(&buf, "-----BEGIN PGP SIGNATURE-----")
	fmt.Fprintln(&buf, "mock-signature")
	fmt.Fprintln(&buf, "-----END PGP SIGNATURE-----")

	return buf.Bytes(), nil
}

func buildAcquireInput(uri, filename string) string {
	return fmt.Sprintf("600 URI Acquire\nURI: %s\nFilename: %s\n\n", uri, filename)
}

func buildTestDeb(t *testing.T, controlContent string) []byte {
	t.Helper()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "./control",
		Size: int64(len(controlContent)),
		Mode: 0644,
	}))
	_, err := tw.Write([]byte(controlContent))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	var gzBuf bytes.Buffer
	gz := gzip.NewWriter(&gzBuf)
	_, err = gz.Write(tarBuf.Bytes())
	require.NoError(t, err)
	require.NoError(t, gz.Close())

	controlTarGz := gzBuf.Bytes()
	debBinary := []byte("2.0\n")

	var buf bytes.Buffer
	buf.WriteString("!<arch>\n")

	writeArEntry := func(name string, data []byte) {
		header := fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n",
			name, "0", "0", "0", "100644", len(data))
		buf.WriteString(header)
		buf.Write(data)

		if len(data)%2 != 0 {
			buf.WriteByte('\n')
		}
	}

	writeArEntry("debian-binary", debBinary)
	writeArEntry("control.tar.gz", controlTarGz)

	return buf.Bytes()
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	amd64ControlV1 := `Package: testpkg
Version: 1.0.0
Architecture: amd64
Maintainer: Test <test@example.com>
Depends: libc6 (>= 2.17), gnupg
Description: Test package
`
	arm64ControlV1 := `Package: testpkg
Version: 1.0.0
Architecture: arm64
Maintainer: Test <test@example.com>
Depends: libc6 (>= 2.17), gnupg
Description: Test package
`
	amd64ControlV09 := `Package: testpkg
Version: 0.9.0
Architecture: amd64
Maintainer: Test <test@example.com>
Description: Test package
`

	amd64DebV1 := buildTestDeb(t, amd64ControlV1)
	arm64DebV1 := buildTestDeb(t, arm64ControlV1)
	amd64DebV09 := buildTestDeb(t, amd64ControlV09)

	releases := []github.Release{
		{
			TagName:     "v1.0.0",
			PublishedAt: "2024-02-01T00:00:00Z",
			Assets: []github.Asset{
				{Name: "testpkg_1.0.0_linux_amd64.deb", Size: int64(len(amd64DebV1))},
				{Name: "testpkg_1.0.0_linux_arm64.deb", Size: int64(len(arm64DebV1))},
				{Name: "testpkg_1.0.0_checksums.txt", Size: 100},
			},
		},
		{
			TagName:     "v0.9.0",
			PublishedAt: "2024-01-01T00:00:00Z",
			Assets: []github.Asset{
				{Name: "testpkg_0.9.0_linux_amd64.deb", Size: int64(len(amd64DebV09))},
			},
		},
	}

	checksums := "abc123def456abcdef1234567890abcdef1234567890abcdef1234567890abcd  testpkg_1.0.0_linux_amd64.deb\nfed987654321fedcba0987654321fedcba0987654321fedcba0987654321fedc  testpkg_1.0.0_linux_arm64.deb\n"

	gitRef := github.GitRef{
		Ref: "refs/tags/v1.0.0",
	}
	gitRef.Object.SHA = "abc123"
	gitRef.Object.Type = "commit"

	gitCommit := github.GitCommit{
		SHA: "abc123",
		Verification: github.Verification{
			Verified: true,
			Reason:   "valid",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/testpkg/releases":
			for ri := range releases {
				for i := range releases[ri].Assets {
					releases[ri].Assets[i].BrowserDownloadURL = fmt.Sprintf("http://%s/download/%s", r.Host, releases[ri].Assets[i].Name)
				}
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(releases)

		case "/repos/owner/testpkg/git/ref/tags/v1.0.0":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(gitRef)

		case "/repos/owner/testpkg/git/commits/abc123":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(gitCommit)

		case "/download/testpkg_1.0.0_checksums.txt":
			w.Write([]byte(checksums))

		case "/download/testpkg_1.0.0_linux_amd64.deb":
			w.Write(amd64DebV1)

		case "/download/testpkg_1.0.0_linux_arm64.deb":
			w.Write(arm64DebV1)

		case "/download/testpkg_0.9.0_linux_amd64.deb":
			w.Write(amd64DebV09)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	t.Cleanup(server.Close)

	return server
}

func newTestServerUnverified(t *testing.T) *httptest.Server {
	t.Helper()

	releases := []github.Release{
		{
			TagName:     "v2.0.0",
			PublishedAt: "2024-01-01T00:00:00Z",
			Assets: []github.Asset{
				{Name: "testpkg_2.0.0_linux_amd64.deb", Size: 1024, BrowserDownloadURL: ""},
			},
		},
	}

	gitRef := github.GitRef{Ref: "refs/tags/v2.0.0"}
	gitRef.Object.SHA = "def456"
	gitRef.Object.Type = "commit"

	gitCommit := github.GitCommit{
		SHA: "def456",
		Verification: github.Verification{
			Verified: false,
			Reason:   "unsigned",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/unsigned/releases":
			for ri := range releases {
				for i := range releases[ri].Assets {
					releases[ri].Assets[i].BrowserDownloadURL = fmt.Sprintf("http://%s/download/%s", r.Host, releases[ri].Assets[i].Name)
				}
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(releases)

		case "/repos/owner/unsigned/git/ref/tags/v2.0.0":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(gitRef)

		case "/repos/owner/unsigned/git/commits/def456":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(gitCommit)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	t.Cleanup(server.Close)

	return server
}

func newTestMethod(t *testing.T, server *httptest.Server) *Method {
	t.Helper()

	cacheDir := filepath.Join(t.TempDir(), "control-cache")
	m := NewWithOptions(&mockSigner{}, cacheDir)
	m.client.BaseURL = server.URL
	m.client.HTTPClient = server.Client()

	return m
}

func TestMethodCapabilities(t *testing.T) {
	server := newTestServer(t)
	m := newTestMethod(t, server)

	var out bytes.Buffer

	m.Run(strings.NewReader(""), &out)

	reader := bufio.NewReader(&out)
	msg, err := ReadMessage(reader)

	require.NoError(t, err)
	assert.Equal(t, 100, msg.Code)
	assert.Equal(t, "Capabilities", msg.Text)
	assert.Equal(t, "1.2", msg.Get("Version"))
}

func TestMethodHandleInRelease(t *testing.T) {
	server := newTestServer(t)
	m := newTestMethod(t, server)

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "InRelease")

	input := buildAcquireInput("github://owner/testpkg/dists/stable/InRelease", filename)
	var out bytes.Buffer

	m.Run(strings.NewReader(input), &out)

	reader := bufio.NewReader(&out)

	_, err := ReadMessage(reader)
	require.NoError(t, err)

	_, err = ReadMessage(reader)
	require.NoError(t, err)

	msg, err := ReadMessage(reader)
	require.NoError(t, err)
	assert.Equal(t, 201, msg.Code)

	content, err := os.ReadFile(filename)
	require.NoError(t, err)

	inReleaseStr := string(content)
	assert.Contains(t, inReleaseStr, "-----BEGIN PGP SIGNED MESSAGE-----")
	assert.Contains(t, inReleaseStr, "Origin: github")
	assert.Contains(t, inReleaseStr, "Label: owner/testpkg")
	assert.Contains(t, inReleaseStr, "-----BEGIN PGP SIGNATURE-----")
}

func TestMethodHandleInReleaseUnverified(t *testing.T) {
	server := newTestServerUnverified(t)
	m := newTestMethod(t, server)

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "InRelease")

	input := buildAcquireInput("github://owner/unsigned/dists/stable/InRelease", filename)
	var out bytes.Buffer

	m.Run(strings.NewReader(input), &out)

	reader := bufio.NewReader(&out)

	_, err := ReadMessage(reader)
	require.NoError(t, err)

	_, err = ReadMessage(reader)
	require.NoError(t, err)

	msg, err := ReadMessage(reader)
	require.NoError(t, err)
	assert.Equal(t, 400, msg.Code)
	assert.Contains(t, msg.Get("Message"), "signature verification failed")
}

func TestMethodHandleInReleaseNoSigner(t *testing.T) {
	server := newTestServer(t)
	cacheDir := filepath.Join(t.TempDir(), "control-cache")
	m := NewWithOptions(nil, cacheDir)
	m.client.BaseURL = server.URL
	m.client.HTTPClient = server.Client()

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "InRelease")

	input := buildAcquireInput("github://owner/testpkg/dists/stable/InRelease", filename)
	var out bytes.Buffer

	m.Run(strings.NewReader(input), &out)

	reader := bufio.NewReader(&out)

	_, err := ReadMessage(reader)
	require.NoError(t, err)

	msg, err := ReadMessage(reader)
	require.NoError(t, err)
	assert.Equal(t, 400, msg.Code)
	assert.Contains(t, msg.Get("Message"), "signing not configured")
}

func TestMethodHandleRelease(t *testing.T) {
	server := newTestServer(t)
	m := newTestMethod(t, server)

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "Release")

	input := buildAcquireInput("github://owner/testpkg/dists/stable/Release", filename)
	var out bytes.Buffer

	m.Run(strings.NewReader(input), &out)

	reader := bufio.NewReader(&out)

	_, err := ReadMessage(reader)
	require.NoError(t, err)

	_, err = ReadMessage(reader)
	require.NoError(t, err)

	msg, err := ReadMessage(reader)
	require.NoError(t, err)
	assert.Equal(t, 201, msg.Code)

	content, err := os.ReadFile(filename)
	require.NoError(t, err)

	releaseStr := string(content)
	assert.Contains(t, releaseStr, "Origin: github")
	assert.Contains(t, releaseStr, "Label: owner/testpkg")
	assert.Contains(t, releaseStr, "Suite: stable")
	assert.Contains(t, releaseStr, "amd64")
	assert.Contains(t, releaseStr, "arm64")
	assert.Contains(t, releaseStr, "SHA256:")
}

func TestMethodHandlePackages(t *testing.T) {
	server := newTestServer(t)
	m := newTestMethod(t, server)

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "Packages")

	input := buildAcquireInput("github://owner/testpkg/dists/stable/main/binary-amd64/Packages", filename)
	var out bytes.Buffer

	m.Run(strings.NewReader(input), &out)

	reader := bufio.NewReader(&out)

	_, err := ReadMessage(reader)
	require.NoError(t, err)

	_, err = ReadMessage(reader)
	require.NoError(t, err)

	msg, err := ReadMessage(reader)
	require.NoError(t, err)
	assert.Equal(t, 201, msg.Code)

	content, err := os.ReadFile(filename)
	require.NoError(t, err)

	pkgStr := string(content)
	assert.Contains(t, pkgStr, "Package: testpkg")
	assert.Contains(t, pkgStr, "Version: 1.0.0")
	assert.Contains(t, pkgStr, "Version: 0.9.0")
	assert.Contains(t, pkgStr, "Architecture: amd64")
	assert.Contains(t, pkgStr, "Depends: libc6 (>= 2.17), gnupg")
	assert.Contains(t, pkgStr, "Maintainer: Test <test@example.com>")
	assert.Contains(t, pkgStr, "pool/v1.0.0/testpkg_1.0.0_linux_amd64.deb")
	assert.Contains(t, pkgStr, "pool/v0.9.0/testpkg_0.9.0_linux_amd64.deb")
	assert.NotContains(t, pkgStr, "arm64")
}

func TestMethodHandlePackagesGz(t *testing.T) {
	server := newTestServer(t)
	m := newTestMethod(t, server)

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "Packages.gz")

	input := buildAcquireInput("github://owner/testpkg/dists/stable/main/binary-amd64/Packages.gz", filename)
	var out bytes.Buffer

	m.Run(strings.NewReader(input), &out)

	reader := bufio.NewReader(&out)

	_, err := ReadMessage(reader)
	require.NoError(t, err)

	_, err = ReadMessage(reader)
	require.NoError(t, err)

	msg, err := ReadMessage(reader)
	require.NoError(t, err)
	assert.Equal(t, 201, msg.Code)
	assert.FileExists(t, filename)
}

func TestMethodHandlePool(t *testing.T) {
	server := newTestServer(t)
	m := newTestMethod(t, server)

	tmpDir := t.TempDir()
	debFilename := filepath.Join(tmpDir, "testpkg.deb")
	pkgFilename := filepath.Join(tmpDir, "Packages")

	input := buildAcquireInput("github://owner/testpkg/dists/stable/main/binary-amd64/Packages", pkgFilename)
	input += buildAcquireInput("github://owner/testpkg/pool/v1.0.0/testpkg_1.0.0_linux_amd64.deb", debFilename)

	var out bytes.Buffer

	m.Run(strings.NewReader(input), &out)

	assert.FileExists(t, debFilename)

	reader := bufio.NewReader(&out)

	// Skip capabilities
	_, err := ReadMessage(reader)
	require.NoError(t, err)

	// Skip status (loadRepo)
	_, err = ReadMessage(reader)
	require.NoError(t, err)

	// Packages done
	msg, err := ReadMessage(reader)
	require.NoError(t, err)
	assert.Equal(t, 201, msg.Code)

	// Pool URI Start
	msg, err = ReadMessage(reader)
	require.NoError(t, err)
	assert.Equal(t, 200, msg.Code)

	// Pool URI Done (served from cache)
	msg, err = ReadMessage(reader)
	require.NoError(t, err)
	assert.Equal(t, 201, msg.Code)
	assert.NotEmpty(t, msg.Get("SHA256-Hash"))
}

func TestMethodHandleReleaseGpg(t *testing.T) {
	server := newTestServer(t)
	m := newTestMethod(t, server)

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "Release.gpg")

	input := buildAcquireInput("github://owner/testpkg/dists/stable/Release.gpg", filename)
	var out bytes.Buffer

	m.Run(strings.NewReader(input), &out)

	reader := bufio.NewReader(&out)

	_, err := ReadMessage(reader)
	require.NoError(t, err)

	msg, err := ReadMessage(reader)
	require.NoError(t, err)
	assert.Equal(t, 400, msg.Code)
}
