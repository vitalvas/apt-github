package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const userAgent = "Mozilla/5.0 (compatible; apt-github/1.0; +https://github.com/vitalvas/apt-github)"

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

func NewClient() *Client {
	return &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    "https://api.github.com",
	}
}

func (c *Client) doGet(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)

	return c.HTTPClient.Do(req)
}

type Release struct {
	TagName     string  `json:"tag_name"`
	PublishedAt string  `json:"published_at"`
	Assets      []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type DebInfo struct {
	Name    string
	Version string
	Arch    string
	Asset   Asset
	SHA256  string
}

func (c *Client) GetLatestRelease(owner, repo string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.BaseURL, owner, repo)

	resp, err := c.doGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func (c *Client) FetchContent(url string) (string, error) {
	resp, err := c.doGet(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func (c *Client) DownloadFile(url, destPath string) (int64, error) {
	resp, err := c.doGet(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)

	return n, err
}

func ParseChecksums(content string) map[string]string {
	result := make(map[string]string)

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 2 {
			result[parts[1]] = parts[0]
		}
	}

	return result
}

func (r *Release) FindChecksumsAsset() *Asset {
	for i, a := range r.Assets {
		if strings.HasSuffix(a.Name, "checksums.txt") {
			return &r.Assets[i]
		}
	}

	return nil
}

func ParseDebFilename(filename, version string) (name, arch string, err error) {
	if !strings.HasSuffix(filename, ".deb") {
		return "", "", fmt.Errorf("not a .deb file: %s", filename)
	}

	base := strings.TrimSuffix(filename, ".deb")

	versionSep := fmt.Sprintf("_%s_", version)
	versionIdx := strings.Index(base, versionSep)
	if versionIdx < 0 {
		return "", "", fmt.Errorf("version %s not found in filename %s", version, filename)
	}

	name = base[:versionIdx]
	rest := base[versionIdx+1+len(version)+1:]

	parts := strings.Split(rest, "_")
	arch = parts[len(parts)-1]

	return name, arch, nil
}

type GitRef struct {
	Ref    string `json:"ref"`
	Object struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	} `json:"object"`
}

type Verification struct {
	Verified bool   `json:"verified"`
	Reason   string `json:"reason"`
}

type GitTag struct {
	SHA          string       `json:"sha"`
	Verification Verification `json:"verification"`
}

type GitCommit struct {
	SHA          string       `json:"sha"`
	Verification Verification `json:"verification"`
}

func (c *Client) VerifyTagSignature(owner, repo, tagName string) (bool, error) {
	refURL := fmt.Sprintf("%s/repos/%s/%s/git/ref/tags/%s", c.BaseURL, owner, repo, tagName)

	resp, err := c.doGet(refURL)
	if err != nil {
		return false, fmt.Errorf("failed to get tag ref: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("tag ref API returned %d", resp.StatusCode)
	}

	var ref GitRef
	if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
		return false, fmt.Errorf("failed to decode tag ref: %w", err)
	}

	switch ref.Object.Type {
	case "tag":
		return c.verifyAnnotatedTag(owner, repo, ref.Object.SHA)
	case "commit":
		return c.verifyCommit(owner, repo, ref.Object.SHA)
	default:
		return false, fmt.Errorf("unexpected ref object type: %s", ref.Object.Type)
	}
}

func (c *Client) verifyAnnotatedTag(owner, repo, sha string) (bool, error) {
	tagURL := fmt.Sprintf("%s/repos/%s/%s/git/tags/%s", c.BaseURL, owner, repo, sha)

	resp, err := c.doGet(tagURL)
	if err != nil {
		return false, fmt.Errorf("failed to get tag object: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("tag object API returned %d", resp.StatusCode)
	}

	var tag GitTag
	if err := json.NewDecoder(resp.Body).Decode(&tag); err != nil {
		return false, fmt.Errorf("failed to decode tag object: %w", err)
	}

	return tag.Verification.Verified, nil
}

func (c *Client) verifyCommit(owner, repo, sha string) (bool, error) {
	commitURL := fmt.Sprintf("%s/repos/%s/%s/git/commits/%s", c.BaseURL, owner, repo, sha)

	resp, err := c.doGet(commitURL)
	if err != nil {
		return false, fmt.Errorf("failed to get commit object: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("commit object API returned %d", resp.StatusCode)
	}

	var commit GitCommit
	if err := json.NewDecoder(resp.Body).Decode(&commit); err != nil {
		return false, fmt.Errorf("failed to decode commit object: %w", err)
	}

	return commit.Verification.Verified, nil
}

func (r *Release) CollectDebInfo(checksums map[string]string) []DebInfo {
	version := strings.TrimPrefix(r.TagName, "v")

	var result []DebInfo

	for _, asset := range r.Assets {
		if !strings.HasSuffix(asset.Name, ".deb") {
			continue
		}

		name, arch, err := ParseDebFilename(asset.Name, version)
		if err != nil {
			continue
		}

		info := DebInfo{
			Name:    name,
			Version: version,
			Arch:    arch,
			Asset:   asset,
		}

		if sha, ok := checksums[asset.Name]; ok {
			info.SHA256 = sha
		}

		result = append(result, info)
	}

	return result
}
