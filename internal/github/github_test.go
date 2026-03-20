package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDebFilename(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		version      string
		expectedName string
		expectedArch string
		expectErr    bool
	}{
		{
			name:         "with os",
			filename:     "systemd-supervisord_1.0.0_linux_amd64.deb",
			version:      "1.0.0",
			expectedName: "systemd-supervisord",
			expectedArch: "amd64",
		},
		{
			name:         "without os",
			filename:     "systemd-supervisord_1.0.0_amd64.deb",
			version:      "1.0.0",
			expectedName: "systemd-supervisord",
			expectedArch: "amd64",
		},
		{
			name:         "arm64 with os",
			filename:     "myapp_2.3.1_linux_arm64.deb",
			version:      "2.3.1",
			expectedName: "myapp",
			expectedArch: "arm64",
		},
		{
			name:      "not a deb",
			filename:  "myapp_1.0.0_linux_amd64.tar.gz",
			version:   "1.0.0",
			expectErr: true,
		},
		{
			name:      "version mismatch",
			filename:  "myapp_1.0.0_amd64.deb",
			version:   "2.0.0",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, arch, err := ParseDebFilename(tt.filename, tt.version)
			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedArch, arch)
		})
	}
}

func TestParseChecksums(t *testing.T) {
	content := `abc123def456  myapp_1.0.0_linux_amd64.deb
789012345678  myapp_1.0.0_linux_arm64.deb
fedcba987654  myapp_1.0.0_checksums.txt
`

	checksums := ParseChecksums(content)

	assert.Len(t, checksums, 3)
	assert.Equal(t, "abc123def456", checksums["myapp_1.0.0_linux_amd64.deb"])
	assert.Equal(t, "789012345678", checksums["myapp_1.0.0_linux_arm64.deb"])
}

func TestReleaseFindChecksumsAsset(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		release := &Release{
			Assets: []Asset{
				{Name: "myapp_1.0.0_linux_amd64.deb"},
				{Name: "myapp_1.0.0_checksums.txt"},
			},
		}

		asset := release.FindChecksumsAsset()
		require.NotNil(t, asset)
		assert.Equal(t, "myapp_1.0.0_checksums.txt", asset.Name)
	})

	t.Run("not found", func(t *testing.T) {
		release := &Release{
			Assets: []Asset{
				{Name: "myapp_1.0.0_linux_amd64.deb"},
			},
		}

		assert.Nil(t, release.FindChecksumsAsset())
	})
}

func TestReleaseCollectDebInfo(t *testing.T) {
	release := &Release{
		TagName: "v1.2.3",
		Assets: []Asset{
			{Name: "myapp_1.2.3_linux_amd64.deb", Size: 1000, BrowserDownloadURL: "https://example.com/amd64.deb"},
			{Name: "myapp_1.2.3_linux_arm64.deb", Size: 900, BrowserDownloadURL: "https://example.com/arm64.deb"},
			{Name: "myapp_1.2.3_checksums.txt", Size: 100},
			{Name: "myapp_1.2.3_linux_amd64.tar.gz", Size: 800},
		},
	}

	checksums := map[string]string{
		"myapp_1.2.3_linux_amd64.deb": "sha256amd64",
		"myapp_1.2.3_linux_arm64.deb": "sha256arm64",
	}

	infos := release.CollectDebInfo(checksums)

	require.Len(t, infos, 2)

	assert.Equal(t, "myapp", infos[0].Name)
	assert.Equal(t, "1.2.3", infos[0].Version)
	assert.Equal(t, "amd64", infos[0].Arch)
	assert.Equal(t, "v1.2.3", infos[0].Tag)
	assert.Equal(t, "sha256amd64", infos[0].SHA256)

	assert.Equal(t, "myapp", infos[1].Name)
	assert.Equal(t, "arm64", infos[1].Arch)
	assert.Equal(t, "v1.2.3", infos[1].Tag)
	assert.Equal(t, "sha256arm64", infos[1].SHA256)
}

func TestClientGetReleases(t *testing.T) {
	releases := []Release{
		{
			TagName: "v1.0.0",
			Assets: []Asset{
				{Name: "test_1.0.0_linux_amd64.deb", Size: 500, BrowserDownloadURL: "https://example.com/test.deb"},
			},
		},
		{
			TagName: "v0.9.0",
			Assets: []Asset{
				{Name: "test_0.9.0_linux_amd64.deb", Size: 400},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/releases", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}

	got, err := client.GetReleases("owner", "repo", 30)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "v1.0.0", got[0].TagName)
	assert.Equal(t, "v0.9.0", got[1].TagName)
}

func TestClientGetReleasesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}

	_, err := client.GetReleases("owner", "repo", 30)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestClientFetchContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("sha256hash  file.deb\n"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}

	content, err := client.FetchContent(fmt.Sprintf("%s/checksums.txt", server.URL))
	require.NoError(t, err)
	assert.Equal(t, "sha256hash  file.deb\n", content)
}

func TestClientFetchBytes(t *testing.T) {
	expectedContent := []byte("binary content here")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(expectedContent)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}

	got, err := client.FetchBytes(fmt.Sprintf("%s/file.deb", server.URL))
	require.NoError(t, err)
	assert.Equal(t, expectedContent, got)
}

func TestClientDownloadFile(t *testing.T) {
	expectedContent := []byte("fake deb content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(expectedContent)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}

	destPath := filepath.Join(t.TempDir(), "test.deb")

	n, err := client.DownloadFile(fmt.Sprintf("%s/test.deb", server.URL), destPath)
	require.NoError(t, err)
	assert.Equal(t, int64(len(expectedContent)), n)

	got, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, got)
}

func TestVerifyTagSignature(t *testing.T) {
	tests := []struct {
		name       string
		refType    string
		verified   bool
		expectedOk bool
		expectErr  bool
	}{
		{
			name:       "verified annotated tag",
			refType:    "tag",
			verified:   true,
			expectedOk: true,
		},
		{
			name:       "unverified annotated tag",
			refType:    "tag",
			verified:   false,
			expectedOk: false,
		},
		{
			name:       "verified lightweight tag",
			refType:    "commit",
			verified:   true,
			expectedOk: true,
		},
		{
			name:       "unverified lightweight tag",
			refType:    "commit",
			verified:   false,
			expectedOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				switch r.URL.Path {
				case "/repos/owner/repo/git/ref/tags/v1.0.0":
					ref := GitRef{Ref: "refs/tags/v1.0.0"}
					ref.Object.SHA = "abc123"
					ref.Object.Type = tt.refType
					json.NewEncoder(w).Encode(ref)

				case "/repos/owner/repo/git/tags/abc123":
					tag := GitTag{
						SHA:          "abc123",
						Verification: Verification{Verified: tt.verified},
					}
					json.NewEncoder(w).Encode(tag)

				case "/repos/owner/repo/git/commits/abc123":
					commit := GitCommit{
						SHA:          "abc123",
						Verification: Verification{Verified: tt.verified},
					}
					json.NewEncoder(w).Encode(commit)

				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := &Client{
				HTTPClient: server.Client(),
				BaseURL:    server.URL,
			}

			ok, err := client.VerifyTagSignature("owner", "repo", "v1.0.0")
			require.NoError(t, err)
			assert.Equal(t, tt.expectedOk, ok)
		})
	}
}

func TestUserAgentHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("User-Agent"), "apt-github")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Release{{TagName: "v1.0.0"}})
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}

	_, err := client.GetReleases("owner", "repo", 30)
	require.NoError(t, err)
}

func TestVerifyTagSignatureRefNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}

	_, err := client.VerifyTagSignature("owner", "repo", "v1.0.0")
	assert.Error(t, err)
}
