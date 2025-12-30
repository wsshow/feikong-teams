package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/wsshow/dl"
	"github.com/wsshow/selfupdate"
)

// Release represents a software version release.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset contains downloadable resource files.
type Asset struct {
	Name               string `json:"name"`
	ContentType        string `json:"content_type"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// IsCompressedFile checks if the file is in compressed format.
func (a Asset) IsCompressedFile() bool {
	return a.ContentType == "application/zip" || a.ContentType == "application/x-gzip"
}

// Updater handles version update checks and operations.
type Updater struct{}

// NewUpdater creates an update handler instance.
func NewUpdater() *Updater {
	return new(Updater)
}

// CheckForUpdates verifies if newer version exists.
func (up Updater) CheckForUpdates(current *semver.Version, owner, repo string) (rel *Release, yes bool, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if !IsHttpSuccess(resp.StatusCode) {
		return nil, false, fmt.Errorf("URL %q is unreachable", url)
	}

	var latest Release
	if err = json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		return nil, false, err
	}

	latestVersion, err := semver.NewVersion(latest.TagName)
	if err != nil {
		return nil, false, err
	}
	if latestVersion.GreaterThan(current) {
		return &latest, true, nil
	}
	return nil, false, nil
}

// Apply performs version update to specified release.
func (up Updater) Apply(rel *Release,
	findAsset func([]Asset) (idx int),
	findChecksum func([]Asset) (algo Algorithm, expectedChecksum string, err error),
) error {
	// findDownloadLink locates asset download URL.
	idx := findAsset(rel.Assets)
	if idx < 0 {
		return ErrAssetNotFound
	}

	// findChecksum verifies file integrity hash.
	algo, expectedChecksum, err := findChecksum(rel.Assets)
	if err != nil {
		return err
	}

	// downloadFile fetches remote resource.
	tmpDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().UnixNano(), 10))
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	url := rel.Assets[idx].BrowserDownloadURL
	srcFilename := filepath.Join(tmpDir, filepath.Base(url))
	dstFilename := srcFilename

	// 创建下载器
	downloader := dl.NewDownloader(url, dl.WithFileName(dstFilename))

	// 设置进度回调
	downloader.OnProgress(func(loaded, total int64, rate string) {
		progress := float64(loaded) / float64(total) * 100
		fmt.Printf("\r进度: %.2f%% | 速度: %s | %d/%d 字节",
			progress, rate, loaded, total)
	})

	// 开始下载
	if err := downloader.Start(); err != nil {
		fmt.Printf("下载失败: %v\n", err)
		return err
	}

	fmt.Println("\n下载完成!")

	// verifyChecksum validates file hash.
	fmt.Println("Computing checksum with", algo)
	if err = VerifyFile(algo, expectedChecksum, srcFilename); err != nil {
		return err
	}
	fmt.Println("Checksums matched")

	// extractFile handles archive decompression.
	if rel.Assets[idx].IsCompressedFile() {
		if dstFilename, err = up.unarchive(srcFilename, tmpDir); err != nil {
			return err
		}
	}

	// updateBinary replaces old executable.
	dstFile, err := os.Open(dstFilename)
	if err != nil {
		return nil
	}
	defer dstFile.Close()
	return selfupdate.Apply(dstFile, selfupdate.Options{})
}

// unarchive extracts compressed files to target directory and returns first extracted file.
func (up Updater) unarchive(srcFile, dstDir string) (dstFile string, err error) {
	if err = Unzip(srcFile, dstDir, nil); err != nil {
		return "", err
	}
	// locateTargetFile finds the main executable after extraction.
	fis, _ := os.ReadDir(dstDir)
	for _, fi := range fis {
		if strings.HasSuffix(srcFile, fi.Name()) {
			continue
		}
		return filepath.Join(dstDir, fi.Name()), nil
	}
	return "", nil
}

// IsHttpSuccess determines if the HTTP status code indicates successful response.
func IsHttpSuccess(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}
