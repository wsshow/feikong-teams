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

// Asset represents a downloadable asset in a release.
type Asset struct {
	Name               string `json:"name"`
	ContentType        string `json:"content_type"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// IsCompressedFile checks if the asset is a compressed file.
func (a Asset) IsCompressedFile() bool {
	return a.ContentType == "application/zip" || a.ContentType == "application/x-gzip"
}

type Updater struct{}

// NewUpdater creates a new Updater instance.
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
	var lastProgress float64
	downloader.OnProgress(func(loaded, total int64, rate string) {
		progress := float64(loaded) / float64(total) * 100
		// 只在进度变化超过0.5%时更新显示
		if progress-lastProgress >= 0.5 || progress >= 100 {
			lastProgress = progress

			// 生成进度条
			barWidth := 40
			filledWidth := int(progress / 100 * float64(barWidth))
			bar := ""
			for i := range barWidth {
				if i < filledWidth {
					bar += "█"
				} else {
					bar += "░"
				}
			}

			// 显示进度
			fmt.Printf("\r[%s] %.2f%% | %s/%s | %s    ",
				bar, progress, formatFileSize(float64(loaded)), formatFileSize(float64(total)), rate)
		}
	})

	// 开始下载
	if err := downloader.Start(); err != nil {
		fmt.Printf("下载失败: %v\n", err)
		return err
	}

	// 校验文件完整性
	fmt.Printf("\n基于 %s 校验文件完整性...\n", algo)
	if err = VerifyFile(algo, expectedChecksum, srcFilename); err != nil {
		return err
	}
	fmt.Printf("文件完整性校验通过\n")

	// 解压缩文件（如果需要）
	if rel.Assets[idx].IsCompressedFile() {
		if dstFilename, err = up.unarchive(srcFilename, tmpDir); err != nil {
			return err
		}
	}

	// 应用更新
	dstFile, err := os.Open(dstFilename)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	return selfupdate.Apply(dstFile, selfupdate.Options{})
}

// unarchive extracts compressed files to target directory and returns first extracted file.
func (up Updater) unarchive(srcFile, dstDir string) (dstFile string, err error) {
	if err = Unzip(srcFile, dstDir, func(processed, total int, fileName string, isDir bool) {
		fmt.Printf("解压中... %d/%d 文件: %s\n", processed, total, fileName)
	}); err != nil {
		return "", err
	}
	// locateTargetFile finds the main executable after extraction.
	fis, _ := os.ReadDir(dstDir)
	for _, fi := range fis {
		if strings.HasSuffix(fi.Name(), ".md") || strings.HasSuffix(fi.Name(), ".zip") {
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

// formatFileSize converts file size in bytes to human-readable string.
func formatFileSize(fileSize float64) string {
	const (
		KB = 1024.0
		MB = KB * 1024.0
		GB = MB * 1024.0
	)

	switch {
	case fileSize >= GB:
		return fmt.Sprintf("%.2f GB", fileSize/GB)
	case fileSize >= MB:
		return fmt.Sprintf("%.2f MB", fileSize/MB)
	case fileSize >= KB:
		return fmt.Sprintf("%.2f KB", fileSize/KB)
	default:
		return fmt.Sprintf("%.2f B", fileSize)
	}
}
