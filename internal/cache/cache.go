package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultBaseDir = "/var/cache/apt-github"
	controlSubdir  = "control"
	releasesSubdir = "releases"
	releasesTTL    = 5 * time.Minute
)

type Entry struct {
	URL    string  `json:"url"`
	Size   int64   `json:"size"`
	Fields []Field `json:"fields"`
}

type ReleasesEntry struct {
	Data      json.RawMessage `json:"data"`
	FetchedAt time.Time       `json:"fetched_at"`
}

type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type DiskCache struct {
	baseDir string
}

func New(baseDir string) *DiskCache {
	return &DiskCache{baseDir: baseDir}
}

func (c *DiskCache) GetControl(url string, size int64) (*Entry, bool) {
	path := c.hashPath(controlSubdir, url)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	if entry.URL != url || entry.Size != size {
		return nil, false
	}

	return &entry, true
}

func (c *DiskCache) PutControl(entry *Entry) error {
	dir := filepath.Join(c.baseDir, controlSubdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return os.WriteFile(c.hashPath(controlSubdir, entry.URL), data, 0644)
}

func (c *DiskCache) GetReleases(key string) (json.RawMessage, bool) {
	path := c.hashPath(releasesSubdir, key)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry ReleasesEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	if time.Since(entry.FetchedAt) > releasesTTL {
		return nil, false
	}

	return entry.Data, true
}

func (c *DiskCache) PutReleases(key string, data json.RawMessage) error {
	dir := filepath.Join(c.baseDir, releasesSubdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	entry := ReleasesEntry{
		Data:      data,
		FetchedAt: time.Now(),
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return os.WriteFile(c.hashPath(releasesSubdir, key), raw, 0644)
}

func (c *DiskCache) Clean() error {
	return os.RemoveAll(c.baseDir)
}

func (c *DiskCache) hashPath(subdir, key string) string {
	hash := sha256.Sum256([]byte(key))

	return filepath.Join(c.baseDir, subdir, fmt.Sprintf("%x.json", hash))
}
