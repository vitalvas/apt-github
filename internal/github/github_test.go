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

func TestAPIErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		expected string
	}{
		{
			name:     "json with message",
			status:   http.StatusForbidden,
			body:     `{"message":"API rate limit exceeded for 1.2.3.4"}`,
			expected: "HTTP 403: API rate limit exceeded for 1.2.3.4",
		},
		{
			name:     "plain text body",
			status:   http.StatusBadGateway,
			body:     "Bad Gateway",
			expected: "HTTP 502: Bad Gateway",
		},
		{
			name:     "empty body",
			status:   http.StatusNotFound,
			body:     "",
			expected: "HTTP 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			recorder.WriteHeader(tt.status)
			recorder.Body.WriteString(tt.body)

			result := apiErrorMessage(recorder.Result())
			assert.Equal(t, tt.expected, result)
		})
	}
}

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
	t.Run("checksums from file", func(t *testing.T) {
		release := &Release{
			TagName: "v1.2.3",
			Assets: []Asset{
				{Name: "myapp_1.2.3_linux_amd64.deb", Size: 1000, Digest: "sha256:digestamd64"},
				{Name: "myapp_1.2.3_linux_arm64.deb", Size: 900, Digest: "sha256:digestarm64"},
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
		assert.Equal(t, "sha256amd64", infos[0].SHA256)
		assert.Equal(t, "sha256arm64", infos[1].SHA256)
	})

	t.Run("fallback to api digest", func(t *testing.T) {
		release := &Release{
			TagName: "v1.2.3",
			Assets: []Asset{
				{Name: "myapp_1.2.3_linux_amd64.deb", Size: 1000, Digest: "sha256:abcdef123456"},
				{Name: "myapp_1.2.3_linux_arm64.deb", Size: 900, Digest: "sha256:789012fedcba"},
			},
		}

		infos := release.CollectDebInfo(nil)

		require.Len(t, infos, 2)
		assert.Equal(t, "abcdef123456", infos[0].SHA256)
		assert.Equal(t, "789012fedcba", infos[1].SHA256)
	})

	t.Run("ignore non-sha256 digest", func(t *testing.T) {
		release := &Release{
			TagName: "v1.2.3",
			Assets: []Asset{
				{Name: "myapp_1.2.3_linux_amd64.deb", Size: 1000, Digest: "md5:abcdef"},
			},
		}

		infos := release.CollectDebInfo(nil)

		require.Len(t, infos, 1)
		assert.Empty(t, infos[0].SHA256)
	})

	t.Run("no digest", func(t *testing.T) {
		release := &Release{
			TagName: "v1.2.3",
			Assets: []Asset{
				{Name: "myapp_1.2.3_linux_amd64.deb", Size: 1000},
			},
		}

		infos := release.CollectDebInfo(nil)

		require.Len(t, infos, 1)
		assert.Empty(t, infos[0].SHA256)
	})

	t.Run("metadata fields", func(t *testing.T) {
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

		assert.Equal(t, "myapp", infos[1].Name)
		assert.Equal(t, "arm64", infos[1].Arch)
		assert.Equal(t, "v1.2.3", infos[1].Tag)
	})
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
	t.Run("with api message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"message": "API rate limit exceeded for 1.2.3.4",
			})
		}))
		defer server.Close()

		client := &Client{
			HTTPClient: server.Client(),
			BaseURL:    server.URL,
		}

		_, err := client.GetReleases("owner", "repo", 30)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "403")
		assert.Contains(t, err.Error(), "API rate limit exceeded")
	})

	t.Run("without body", func(t *testing.T) {
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
	})
}

func TestClientFetchAssetContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("sha256hash  file.deb\n"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}

	asset := Asset{
		Name:               "checksums.txt",
		BrowserDownloadURL: fmt.Sprintf("%s/checksums.txt", server.URL),
	}

	content, err := client.FetchAssetContent("owner", "repo", asset)
	require.NoError(t, err)
	assert.Equal(t, "sha256hash  file.deb\n", content)
}

func TestClientFetchAssetBytes(t *testing.T) {
	expectedContent := []byte("binary content here")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(expectedContent)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}

	asset := Asset{
		Name:               "file.deb",
		BrowserDownloadURL: fmt.Sprintf("%s/file.deb", server.URL),
	}

	got, err := client.FetchAssetBytes("owner", "repo", asset)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, got)
}

func TestClientDownloadAssetFile(t *testing.T) {
	expectedContent := []byte("fake deb content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(expectedContent)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}

	asset := Asset{
		Name:               "test.deb",
		BrowserDownloadURL: fmt.Sprintf("%s/test.deb", server.URL),
	}

	destPath := filepath.Join(t.TempDir(), "test.deb")

	n, err := client.DownloadAssetFile("owner", "repo", asset, destPath)
	require.NoError(t, err)
	assert.Equal(t, int64(len(expectedContent)), n)

	got, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, got)
}

func TestClientFetchAssetWithToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/octet-stream", r.Header.Get("Accept"))
		assert.Equal(t, "/api-asset-url", r.URL.Path)
		w.Write([]byte("authenticated content"))
	}))
	defer server.Close()

	tokenDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "default"), []byte("test-token"), 0600))

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		TokensDir:  tokenDir,
	}

	asset := Asset{
		Name:               "file.deb",
		URL:                fmt.Sprintf("%s/api-asset-url", server.URL),
		BrowserDownloadURL: fmt.Sprintf("%s/browser-url", server.URL),
	}

	got, err := client.FetchAssetBytes("owner", "repo", asset)
	require.NoError(t, err)
	assert.Equal(t, []byte("authenticated content"), got)
}

func TestClientFetchAssetWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Equal(t, "/browser-url", r.URL.Path)
		w.Write([]byte("public content"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		TokensDir:  t.TempDir(),
	}

	asset := Asset{
		Name:               "file.deb",
		URL:                fmt.Sprintf("%s/api-asset-url", server.URL),
		BrowserDownloadURL: fmt.Sprintf("%s/browser-url", server.URL),
	}

	got, err := client.FetchAssetBytes("owner", "repo", asset)
	require.NoError(t, err)
	assert.Equal(t, []byte("public content"), got)
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
		assert.Contains(t, r.Header.Get("User-Agent"), "apt-transport-github")
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

func TestAuthTokenHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer ghp_testtoken123", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Release{{TagName: "v1.0.0"}})
	}))
	defer server.Close()

	tokenDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "default"), []byte("ghp_testtoken123"), 0600))

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		TokensDir:  tokenDir,
	}

	_, err := client.GetReleases("owner", "repo", 30)
	require.NoError(t, err)
}

func TestNoAuthHeaderWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Release{{TagName: "v1.0.0"}})
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		TokensDir:  t.TempDir(),
	}

	_, err := client.GetReleases("owner", "repo", 30)
	require.NoError(t, err)
}

func TestResolveToken(t *testing.T) {
	t.Run("repo token", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")

		tokenDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "default"), []byte("default-token"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "repo_vitalvas"), []byte("owner-token"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "repo_vitalvas__myapp"), []byte("repo-token"), 0600))

		client := &Client{TokensDir: tokenDir}
		assert.Equal(t, "repo-token", client.resolveToken("vitalvas", "myapp"))
	})

	t.Run("owner token", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")

		tokenDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "default"), []byte("default-token"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "repo_vitalvas"), []byte("owner-token"), 0600))

		client := &Client{TokensDir: tokenDir}
		assert.Equal(t, "owner-token", client.resolveToken("vitalvas", "other-repo"))
	})

	t.Run("default token", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")

		tokenDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "default"), []byte("default-token"), 0600))

		client := &Client{TokensDir: tokenDir}
		assert.Equal(t, "default-token", client.resolveToken("other-owner", "repo"))
	})

	t.Run("env var fallback", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "env-token")

		client := &Client{TokensDir: t.TempDir()}
		assert.Equal(t, "env-token", client.resolveToken("owner", "repo"))
	})

	t.Run("no token", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")

		client := &Client{TokensDir: t.TempDir()}
		assert.Empty(t, client.resolveToken("owner", "repo"))
	})

	t.Run("whitespace trimmed", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")

		tokenDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "default"), []byte("  my-token\n"), 0600))

		client := &Client{TokensDir: tokenDir}
		assert.Equal(t, "my-token", client.resolveToken("owner", "repo"))
	})
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
