package update

import (
	"bufio"
	"fkteams/version"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"
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
		fmt.Printf("You are up to date! fkteams v%s is the latest version.\n", info.Version)
		return nil
	}
	fmt.Printf("A new version of fkteams(%s) is available\n", latest.TagName)

	// 应用更新
	if err = up.Apply(latest, findAsset, findChecksum); err != nil {
		return err
	}
	fmt.Println("Update completed")
	return nil
}

func findAsset(items []Asset) (idx int) {
	ext := "zip"
	suffix := fmt.Sprintf("%s_%s.%s", runtime.GOOS, runtime.GOARCH, ext)
	for i := range items {
		if strings.HasSuffix(items[i].BrowserDownloadURL, suffix) {
			return i
		}
	}
	return -1
}

func findChecksum(items []Asset) (algo Algorithm, expectedChecksum string, err error) {
	ext := "zip"
	suffix := fmt.Sprintf("%s_%s.%s", runtime.GOOS, runtime.GOARCH, ext)
	var checksumFileURL string
	for i := range items {
		if items[i].Name == "sha256sum.txt" {
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
