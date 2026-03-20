package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControlCache(t *testing.T) {
	t.Run("put and get", func(t *testing.T) {
		c := New(t.TempDir())

		entry := &Entry{
			Fields: []Field{
				{Key: "Package", Value: "testpkg"},
				{Key: "Depends", Value: "libc6"},
			},
		}

		require.NoError(t, c.PutControl("owner", "repo", "v1.0.0", "testpkg_1.0.0_amd64.deb", entry))

		got, ok := c.GetControl("owner", "repo", "v1.0.0", "testpkg_1.0.0_amd64.deb")
		require.True(t, ok)
		assert.Equal(t, "testpkg", got.Fields[0].Value)
		assert.Equal(t, "libc6", got.Fields[1].Value)
	})

	t.Run("stores at expected path", func(t *testing.T) {
		base := t.TempDir()
		c := New(base)

		entry := &Entry{Fields: []Field{{Key: "Package", Value: "testpkg"}}}
		require.NoError(t, c.PutControl("owner", "repo", "v1.0.0", "testpkg_1.0.0_amd64.deb", entry))

		expected := filepath.Join(base, "owner", "repo", "v1.0.0", "testpkg_1.0.0_amd64.json")
		assert.FileExists(t, expected)
	})

	t.Run("multiple packages per tag", func(t *testing.T) {
		c := New(t.TempDir())

		minion := &Entry{Fields: []Field{{Key: "Package", Value: "salt-minion"}}}
		master := &Entry{Fields: []Field{{Key: "Package", Value: "salt-master"}}}

		require.NoError(t, c.PutControl("owner", "repo", "v1.0.0", "salt-minion_1.0.0_amd64.deb", minion))
		require.NoError(t, c.PutControl("owner", "repo", "v1.0.0", "salt-master_1.0.0_amd64.deb", master))

		gotMinion, ok := c.GetControl("owner", "repo", "v1.0.0", "salt-minion_1.0.0_amd64.deb")
		require.True(t, ok)
		assert.Equal(t, "salt-minion", gotMinion.Fields[0].Value)

		gotMaster, ok := c.GetControl("owner", "repo", "v1.0.0", "salt-master_1.0.0_amd64.deb")
		require.True(t, ok)
		assert.Equal(t, "salt-master", gotMaster.Fields[0].Value)
	})

	t.Run("miss on unknown filename", func(t *testing.T) {
		c := New(t.TempDir())

		_, ok := c.GetControl("owner", "repo", "v9.9.9", "missing_1.0.0_amd64.deb")
		assert.False(t, ok)
	})

	t.Run("overwrite existing", func(t *testing.T) {
		c := New(t.TempDir())

		entry1 := &Entry{Fields: []Field{{Key: "Version", Value: "1.0"}}}
		entry2 := &Entry{Fields: []Field{{Key: "Version", Value: "2.0"}}}

		require.NoError(t, c.PutControl("owner", "repo", "v1.0.0", "pkg_1.0.0_amd64.deb", entry1))
		require.NoError(t, c.PutControl("owner", "repo", "v1.0.0", "pkg_1.0.0_amd64.deb", entry2))

		got, ok := c.GetControl("owner", "repo", "v1.0.0", "pkg_1.0.0_amd64.deb")
		require.True(t, ok)
		assert.Equal(t, "2.0", got.Fields[0].Value)
	})
}

func TestControlFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "standard deb", input: "testpkg_1.0.0_amd64.deb", expected: "testpkg_1.0.0_amd64.json"},
		{name: "no deb extension", input: "testpkg_1.0.0_amd64", expected: "testpkg_1.0.0_amd64.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, controlFilename(tt.input))
		})
	}
}

func TestReleasesCache(t *testing.T) {
	t.Run("put and get", func(t *testing.T) {
		c := New(t.TempDir())

		data := json.RawMessage(`[{"tag_name":"v1.0.0"}]`)

		require.NoError(t, c.PutReleases("owner", "repo", data))

		got, ok := c.GetReleases("owner", "repo")
		require.True(t, ok)
		assert.JSONEq(t, `[{"tag_name":"v1.0.0"}]`, string(got))
	})

	t.Run("stores at expected path", func(t *testing.T) {
		base := t.TempDir()
		c := New(base)

		require.NoError(t, c.PutReleases("owner", "repo", json.RawMessage(`[]`)))

		expected := filepath.Join(base, "owner", "repo", "releases.json")
		assert.FileExists(t, expected)
	})

	t.Run("miss on unknown repo", func(t *testing.T) {
		c := New(t.TempDir())

		_, ok := c.GetReleases("owner", "missing")
		assert.False(t, ok)
	})

	t.Run("expired entry", func(t *testing.T) {
		base := t.TempDir()
		c := New(base)

		entry := ReleasesEntry{
			Data:      json.RawMessage(`[]`),
			FetchedAt: time.Now().Add(-10 * time.Minute),
		}

		dir := filepath.Join(base, "owner", "repo")
		require.NoError(t, os.MkdirAll(dir, 0755))

		raw, err := json.Marshal(entry)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "releases.json"), raw, 0644))

		_, ok := c.GetReleases("owner", "repo")
		assert.False(t, ok)
	})
}

func TestPackageCache(t *testing.T) {
	t.Run("put and get", func(t *testing.T) {
		c := New(t.TempDir())

		debData := []byte("fake deb content")

		path, err := c.PutPackage("owner", "repo", "v1.0.0", "test_1.0.0_amd64.deb", debData)
		require.NoError(t, err)
		assert.Contains(t, path, "owner/repo/v1.0.0/test_1.0.0_amd64.deb")

		cachedPath, ok := c.GetPackage("owner", "repo", "v1.0.0", "test_1.0.0_amd64.deb")
		require.True(t, ok)
		assert.Equal(t, path, cachedPath)

		got, err := os.ReadFile(cachedPath)
		require.NoError(t, err)
		assert.Equal(t, debData, got)
	})

	t.Run("miss on unknown path", func(t *testing.T) {
		c := New(t.TempDir())

		_, ok := c.GetPackage("owner", "repo", "v1.0.0", "missing.deb")
		assert.False(t, ok)
	})
}

func TestCleanStaleTags(t *testing.T) {
	t.Run("removes stale keeps valid", func(t *testing.T) {
		c := New(t.TempDir())

		_, err := c.PutPackage("owner", "repo", "v1.0.0", "pkg_1.0.0_amd64.deb", []byte("v1"))
		require.NoError(t, err)

		_, err = c.PutPackage("owner", "repo", "v0.9.0", "pkg_0.9.0_amd64.deb", []byte("v09"))
		require.NoError(t, err)

		_, err = c.PutPackage("owner", "repo", "v0.8.0", "pkg_0.8.0_amd64.deb", []byte("v08"))
		require.NoError(t, err)

		validTags := map[string]bool{
			"v1.0.0": true,
			"v0.9.0": true,
		}

		require.NoError(t, c.CleanStaleTags("owner", "repo", validTags))

		_, ok := c.GetPackage("owner", "repo", "v1.0.0", "pkg_1.0.0_amd64.deb")
		assert.True(t, ok)

		_, ok = c.GetPackage("owner", "repo", "v0.9.0", "pkg_0.9.0_amd64.deb")
		assert.True(t, ok)

		_, ok = c.GetPackage("owner", "repo", "v0.8.0", "pkg_0.8.0_amd64.deb")
		assert.False(t, ok)
	})

	t.Run("no dir exists", func(t *testing.T) {
		c := New(t.TempDir())

		err := c.CleanStaleTags("owner", "nonexistent", map[string]bool{})
		assert.NoError(t, err)
	})
}

func TestClean(t *testing.T) {
	c := New(t.TempDir())

	require.NoError(t, c.PutControl("owner", "repo", "v1.0.0", "test_1.0.0_amd64.deb", &Entry{
		Fields: []Field{{Key: "Package", Value: "test"}},
	}))

	require.NoError(t, c.PutReleases("owner", "repo", json.RawMessage(`[]`)))

	require.NoError(t, c.Clean())

	_, ok := c.GetControl("owner", "repo", "v1.0.0", "test_1.0.0_amd64.deb")
	assert.False(t, ok)

	_, ok = c.GetReleases("owner", "repo")
	assert.False(t, ok)
}
