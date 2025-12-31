package update

import (
	"bufio"
	"fkteams/version"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"unicode"

	"github.com/Masterminds/semver/v3"
	"github.com/pterm/pterm"
)

func SelfUpdate(owner string, repo string) (err error) {
	up := NewUpdater()
	info := version.Get()
	// 检查更新
	latest, yes, err := up.CheckForUpdates(semver.MustParse(info.Version), owner, repo)
	if err != nil {
		return err
	}
	if !yes {
		pterm.Info.Printfln("当前已是最新版本: %s", info.Version)
		return nil
	}

	pterm.Info.Printfln("发现新版本: %s，正在下载更新...", latest.TagName)

	// 应用更新
	if err = up.Apply(latest, findAsset, findChecksum); err != nil {
		return err
	}
	pterm.Success.Printfln("版本升级成功，当前版本: %s", latest.TagName)
	return nil
}

func findAsset(items []Asset) (idx int) {
	ext := "zip"
	suffix := fmt.Sprintf("%s_%s.%s", CapitalizeOS(), GetNormalizedArch(), ext)
	for i := range items {
		if strings.HasSuffix(items[i].BrowserDownloadURL, suffix) {
			return i
		}
	}
	return -1
}

func findChecksum(items []Asset) (algo Algorithm, expectedChecksum string, err error) {
	ext := "zip"
	suffix := fmt.Sprintf("%s_%s.%s", CapitalizeOS(), GetNormalizedArch(), ext)
	var checksumFileURL string
	for i := range items {
		if items[i].Name == "checksums.txt" {
			checksumFileURL = items[i].BrowserDownloadURL
			break
		}
	}
	if checksumFileURL == "" {
		return SHA256, "", ErrChecksumFileNotFound
	}

	resp, err := http.Get(checksumFileURL)
	if err != nil {
		return SHA256, "", err
	}
	defer resp.Body.Close()

	if !IsHttpSuccess(resp.StatusCode) {
		return "", "", fmt.Errorf("URL %q is unreachable", checksumFileURL)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasSuffix(line, suffix) {
			continue
		}
		return SHA256, strings.Fields(line)[0], nil
	}
	if err = scanner.Err(); err != nil {
		return SHA256, "", err
	}
	return SHA256, "", ErrChecksumFileNotFound
}

// CapitalizeOS 将运行时的操作系统名称首字母大写
func CapitalizeOS() string {
	osName := runtime.GOOS
	if len(osName) == 0 {
		return ""
	}

	runes := []rune(osName)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}

// GetNormalizedArch 获取格式化后的架构名称
func GetNormalizedArch() string {
	arch := runtime.GOARCH

	switch arch {
	case "amd64":
		return "x86_64"
	case "386":
		return "i386"
	default:
		// 对于 arm64, armv7 等，通常保持原样
		return arch
	}
}
