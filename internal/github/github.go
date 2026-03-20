package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

var (
	tokenFile  = "/etc/apt-transport-github/token"
	appVersion = "dev"
)

func SetVersion(v string) {
	appVersion = v
}

func userAgent() string {
	return fmt.Sprintf("Mozilla/5.0 (compatible; apt-transport-github/%s; +https://github.com/vitalvas/apt-transport-github)", appVersion)
}

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	Token      string
}

func NewClient() *Client {
	return &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    "https://api.github.com",
		Token:      loadToken(),
	}
}

func loadToken() string {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}

	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

func apiErrorMessage(resp *http.Response) string {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	var apiErr struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, apiErr.Message)
	}

	if len(body) > 0 {
		text := strings.TrimSpace(string(body))
		if len(text) > 200 {
			text = text[:200]
		}

		return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, text)
	}

	return fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func (c *Client) doGet(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent())

	if c.Token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	}

	return c.HTTPClient.Do(req)
}

func (c *Client) downloadURL(asset Asset) string {
	if c.Token != "" && asset.URL != "" {
		return asset.URL
	}

	return asset.BrowserDownloadURL
}

func (c *Client) doGetAsset(asset Asset) (*http.Response, error) {
	url := c.downloadURL(asset)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent())

	if c.Token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Token))
		req.Header.Set("Accept", "application/octet-stream")
	}

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
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

type DebInfo struct {
	Name    string
	Version string
	Arch    string
	Tag     string
	Asset   Asset
	SHA256  string
	Control []ControlField
}

type ControlField struct {
	Key   string
	Value string
}

func (c *Client) GetReleases(owner, repo string, limit int) ([]Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d", c.BaseURL, owner, repo, limit)

	resp, err := c.doGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", apiErrorMessage(resp))
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	return releases, nil
}

func (c *Client) FetchAssetContent(asset Asset) (string, error) {
	resp, err := c.doGetAsset(asset)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch returned %s", apiErrorMessage(resp))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func (c *Client) FetchAssetBytes(asset Asset) ([]byte, error) {
	resp, err := c.doGetAsset(asset)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch returned %s", apiErrorMessage(resp))
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) DownloadAssetFile(asset Asset, destPath string) (int64, error) {
	resp, err := c.doGetAsset(asset)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download returned %s", apiErrorMessage(resp))
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
		return false, fmt.Errorf("tag ref API returned %s", apiErrorMessage(resp))
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
		return false, fmt.Errorf("tag object API returned %s", apiErrorMessage(resp))
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
		return false, fmt.Errorf("commit object API returned %s", apiErrorMessage(resp))
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
			Tag:     r.TagName,
			Asset:   asset,
		}

		if sha, ok := checksums[asset.Name]; ok {
			info.SHA256 = sha
		} else if strings.HasPrefix(asset.Digest, "sha256:") {
			info.SHA256 = strings.TrimPrefix(asset.Digest, "sha256:")
		}

		result = append(result, info)
	}

	return result
}
