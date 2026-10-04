package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"jacob/internal/buildinfo"
)

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

type releaseAPI struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	HTMLURL     string  `json:"html_url"`
	PublishedAt string  `json:"published_at"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	Assets      []Asset `json:"assets"`
}

type Info struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Available      bool   `json:"available"`
	ReleaseName    string `json:"releaseName,omitempty"`
	ReleaseURL     string `json:"releaseURL,omitempty"`
	PublishedAt    string `json:"publishedAt,omitempty"`
	Prerelease     bool   `json:"prerelease,omitempty"`
	Notes          string `json:"notes,omitempty"`
	Asset          *Asset `json:"asset,omitempty"`
	Installable    bool   `json:"installable"`
	InstallNote    string `json:"installNote,omitempty"`
}

type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) Check(ctx context.Context) (Info, error) {
	url := "https://api.github.com/repos/" + buildinfo.Repository + "/releases?per_page=30"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Info{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "JACoB/"+buildinfo.Version)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.http.Do(req)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Info{}, fmt.Errorf("GitHub release check returned %s", resp.Status)
	}
	var releases []releaseAPI
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&releases); err != nil {
		return Info{}, err
	}
	candidates := make([]releaseAPI, 0, len(releases))
	for _, rel := range releases {
		if rel.Draft || strings.TrimSpace(rel.TagName) == "" {
			continue
		}
		candidates = append(candidates, rel)
	}
	if len(candidates) == 0 {
		return Info{CurrentVersion: buildinfo.Version, LatestVersion: buildinfo.Version, Available: false, Installable: false, InstallNote: "No published JACoB releases were found."}, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return buildinfo.CompareVersion(candidates[i].TagName, candidates[j].TagName) > 0
	})
	latest := candidates[0]
	asset := selectAsset(latest.Assets)
	info := Info{
		CurrentVersion: buildinfo.Version,
		LatestVersion:  strings.TrimPrefix(strings.TrimPrefix(latest.TagName, "v"), "V"),
		Available:      buildinfo.CompareVersion(latest.TagName, buildinfo.Version) > 0,
		ReleaseName:    latest.Name, ReleaseURL: latest.HTMLURL, PublishedAt: latest.PublishedAt,
		Prerelease: latest.Prerelease, Notes: latest.Body, Asset: asset,
	}
	if asset == nil {
		info.Installable = false
		info.InstallNote = "No update package matches this operating system and architecture."
	} else {
		switch runtime.GOOS {
		case "windows", "linux":
			info.Installable = true
		default:
			info.Installable = false
			info.InstallNote = "Automatic installation is not available on this operating system."
		}
	}
	return info, nil
}

func selectAsset(assets []Asset) *Asset {
	want := func(name string) bool {
		n := strings.ToLower(name)
		switch runtime.GOOS {
		case "windows":
			if !strings.Contains(n, "jacob") || !strings.HasSuffix(n, ".exe") {
				return false
			}
			if strings.Contains(n, "portable") || strings.Contains(n, "no-recorder") {
				return false
			}
			return strings.Contains(n, "setup") || strings.Contains(n, "alpha-")
		case "linux":
			return strings.Contains(n, "jacob") && strings.Contains(n, "linux") && strings.Contains(n, runtime.GOARCH)
		default:
			return false
		}
	}
	for i := range assets {
		if want(assets[i].Name) {
			a := assets[i]
			return &a
		}
	}
	return nil
}

func (c *Client) Download(ctx context.Context, asset Asset, dir string) (string, error) {
	if strings.TrimSpace(asset.BrowserDownloadURL) == "" {
		return "", errors.New("release asset has no download URL")
	}
	if asset.Size > 150<<20 {
		return "", errors.New("release asset is unexpectedly large")
	}
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := filepath.Base(asset.Name)
	if name == "." || name == "" {
		name = "jacob-update.bin"
	}
	path := filepath.Join(dir, name)
	tmp := path + ".download"
	_ = os.Remove(tmp)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "JACoB/"+buildinfo.Version)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update download returned %s", resp.Status)
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 150<<20+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", closeErr
	}
	if written > 150<<20 {
		_ = os.Remove(tmp)
		return "", errors.New("update download exceeded size limit")
	}
	if asset.Size > 0 && written != asset.Size {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("update size mismatch: got %d bytes, expected %d", written, asset.Size)
	}
	if d := strings.TrimSpace(asset.Digest); d != "" {
		want := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(d), "sha256:"))
		got := hex.EncodeToString(h.Sum(nil))
		if want != "" && want != got {
			_ = os.Remove(tmp)
			return "", errors.New("update checksum did not match the GitHub release asset")
		}
	}
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(path, 0o755)
	}
	return path, nil
}

// ReplaceLinuxExecutable swaps the running Linux binary with the downloaded update,
// starts the replacement, and leaves a .previous file beside it for recovery.
func ReplaceLinuxExecutable(downloadPath string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("linux replacement requested on another operating system")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	st, err := os.Stat(exe)
	if err != nil {
		return "", err
	}
	newPath := exe + ".new"
	backup := exe + ".previous"
	_ = os.Remove(newPath)
	_ = os.Remove(backup)
	in, err := os.Open(downloadPath)
	if err != nil {
		return "", err
	}
	out, err := os.OpenFile(newPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, st.Mode().Perm())
	if err != nil {
		_ = in.Close()
		return "", err
	}
	_, copyErr := io.Copy(out, in)
	closeOut := out.Close()
	_ = in.Close()
	if copyErr != nil {
		_ = os.Remove(newPath)
		return "", copyErr
	}
	if closeOut != nil {
		_ = os.Remove(newPath)
		return "", closeOut
	}
	if err := os.Chmod(newPath, st.Mode().Perm()|0o111); err != nil {
		_ = os.Remove(newPath)
		return "", err
	}
	if err := os.Rename(exe, backup); err != nil {
		_ = os.Remove(newPath)
		return "", fmt.Errorf("cannot replace current executable: %w", err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = os.Rename(backup, exe)
		_ = os.Remove(newPath)
		return "", fmt.Errorf("cannot install replacement executable: %w", err)
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		_ = os.Remove(exe)
		_ = os.Rename(backup, exe)
		return "", fmt.Errorf("updated executable could not be launched: %w", err)
	}
	return backup, nil
}
