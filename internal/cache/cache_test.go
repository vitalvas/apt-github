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
			URL:  "https://example.com/test.deb",
			Size: 1024,
			Fields: []Field{
				{Key: "Package", Value: "testpkg"},
				{Key: "Depends", Value: "libc6"},
			},
		}

		require.NoError(t, c.PutControl(entry))

		got, ok := c.GetControl("https://example.com/test.deb", 1024)
		require.True(t, ok)
		assert.Equal(t, "testpkg", got.Fields[0].Value)
		assert.Equal(t, "libc6", got.Fields[1].Value)
	})

	t.Run("miss on unknown url", func(t *testing.T) {
		c := New(t.TempDir())

		_, ok := c.GetControl("https://example.com/missing.deb", 100)
		assert.False(t, ok)
	})

	t.Run("miss on size mismatch", func(t *testing.T) {
		c := New(t.TempDir())

		entry := &Entry{
			URL:    "https://example.com/test.deb",
			Size:   1024,
			Fields: []Field{{Key: "Package", Value: "testpkg"}},
		}

		require.NoError(t, c.PutControl(entry))

		_, ok := c.GetControl("https://example.com/test.deb", 2048)
		assert.False(t, ok)
	})

	t.Run("overwrite existing", func(t *testing.T) {
		c := New(t.TempDir())

		entry1 := &Entry{
			URL:    "https://example.com/test.deb",
			Size:   1024,
			Fields: []Field{{Key: "Version", Value: "1.0"}},
		}

		entry2 := &Entry{
			URL:    "https://example.com/test.deb",
			Size:   2048,
			Fields: []Field{{Key: "Version", Value: "2.0"}},
		}

		require.NoError(t, c.PutControl(entry1))
		require.NoError(t, c.PutControl(entry2))

		got, ok := c.GetControl("https://example.com/test.deb", 2048)
		require.True(t, ok)
		assert.Equal(t, "2.0", got.Fields[0].Value)
	})
}

func TestReleasesCache(t *testing.T) {
	t.Run("put and get", func(t *testing.T) {
		c := New(t.TempDir())

		data := json.RawMessage(`[{"tag_name":"v1.0.0"}]`)

		require.NoError(t, c.PutReleases("owner/repo", data))

		got, ok := c.GetReleases("owner/repo")
		require.True(t, ok)
		assert.JSONEq(t, `[{"tag_name":"v1.0.0"}]`, string(got))
	})

	t.Run("miss on unknown key", func(t *testing.T) {
		c := New(t.TempDir())

		_, ok := c.GetReleases("owner/missing")
		assert.False(t, ok)
	})

	t.Run("expired entry", func(t *testing.T) {
		c := New(t.TempDir())

		entry := ReleasesEntry{
			Data:      json.RawMessage(`[]`),
			FetchedAt: time.Now().Add(-10 * time.Minute),
		}

		dir := filepath.Join(c.baseDir, releasesSubdir)
		require.NoError(t, os.MkdirAll(dir, 0755))

		raw, err := json.Marshal(entry)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(c.hashPath(releasesSubdir, "owner/repo"), raw, 0644))

		_, ok := c.GetReleases("owner/repo")
		assert.False(t, ok)
	})
}

func TestPackageCache(t *testing.T) {
	t.Run("put and get", func(t *testing.T) {
		c := New(t.TempDir())

		debData := []byte("fake deb content")
		url := "https://example.com/test_1.0_amd64.deb"

		path, err := c.PutPackage(url, debData)
		require.NoError(t, err)
		assert.Contains(t, path, ".deb")

		cachedPath, ok := c.GetPackage(url)
		require.True(t, ok)
		assert.Equal(t, path, cachedPath)

		got, err := os.ReadFile(cachedPath)
		require.NoError(t, err)
		assert.Equal(t, debData, got)
	})

	t.Run("miss on unknown url", func(t *testing.T) {
		c := New(t.TempDir())

		_, ok := c.GetPackage("https://example.com/missing.deb")
		assert.False(t, ok)
	})
}

func TestClean(t *testing.T) {
	c := New(t.TempDir())

	require.NoError(t, c.PutControl(&Entry{
		URL:    "https://example.com/test.deb",
		Size:   100,
		Fields: []Field{{Key: "Package", Value: "test"}},
	}))

	require.NoError(t, c.PutReleases("owner/repo", json.RawMessage(`[]`)))

	require.NoError(t, c.Clean())

	_, ok := c.GetControl("https://example.com/test.deb", 100)
	assert.False(t, ok)

	_, ok = c.GetReleases("owner/repo")
	assert.False(t, ok)
}

func TestHashPath(t *testing.T) {
	c := New("/tmp/cache")

	path1 := c.hashPath("control", "https://example.com/a.deb")
	path2 := c.hashPath("control", "https://example.com/b.deb")

	assert.NotEqual(t, path1, path2)
	assert.Contains(t, path1, "/tmp/cache/control/")
	assert.Contains(t, path1, ".json")
}
