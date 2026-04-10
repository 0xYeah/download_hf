// Package update actions/update/update.go
package update

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

var apiClient = &http.Client{Timeout: 30 * time.Second}

// Command returns the update subcommand, wiring in the current binary version.
func Command(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "升级到最新版本",
		Run: func(cmd *cobra.Command, args []string) {
			run(version)
		},
	}
}

func run(currentVersion string) {
	latest, err := fetchLatestVersion()
	if err != nil {
		fmt.Printf("❌ 获取最新版本失败：%v\n", err)
		os.Exit(1)
	}

	if currentVersion != "" && currentVersion == latest {
		fmt.Printf("✅ 已是最新版本 %s\n", currentVersion)
		return
	}

	if currentVersion != "" {
		fmt.Printf("🔄 升级 %s -> %s\n", currentVersion, latest)
	} else {
		fmt.Printf("🔄 安装最新版本 %s\n", latest)
	}

	os_, arch := detectPlatform()
	zipName := fmt.Sprintf("download_hf_release_%s_%s_%s.zip", latest, os_, arch)
	url := fmt.Sprintf("https://github.com/0xYeah/download_hf/releases/download/%s/%s", latest, zipName)

	tmpDir, err := os.MkdirTemp("", "download_hf_update_*")
	if err != nil {
		fmt.Printf("❌ 创建临时目录失败：%v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, zipName)
	fmt.Printf("⏳ 下载 %s\n", zipName)
	if err := downloadToFile(url, zipPath); err != nil {
		fmt.Printf("❌ 下载失败：%v\n", err)
		os.Exit(1)
	}

	newBin, err := extractBinary(zipPath, tmpDir)
	if err != nil {
		fmt.Printf("❌ 解压失败：%v\n", err)
		os.Exit(1)
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("❌ 获取当前路径失败：%v\n", err)
		os.Exit(1)
	}

	if err := replaceBinary(newBin, execPath); err != nil {
		fmt.Printf("❌ 替换失败：%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 已升级到 %s\n", latest)
}

type ghRelease struct {
	TagName string `json:"tag_name"`
}

func detectPlatform() (os_, arch string) {
	os_ = runtime.GOOS
	switch os_ {
	case "darwin":
		arch = "universal"
	case "linux", "windows":
		switch runtime.GOARCH {
		case "amd64":
			arch = "amd64"
		case "arm64":
			arch = "arm64"
		default:
			fmt.Printf("❌ 不支持的架构：%s\n", runtime.GOARCH)
			os.Exit(1)
		}
	default:
		fmt.Printf("❌ 不支持的系统：%s\n", os_)
		os.Exit(1)
	}
	return
}

func fetchLatestVersion() (string, error) {
	resp, err := apiClient.Get("https://api.github.com/repos/0xYeah/download_hf/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.TagName == "" {
		return "", fmt.Errorf("无法解析版本号")
	}
	return r.TagName, nil
}

func downloadToFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func extractBinary(zipPath, destDir string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	binName := "download_hf"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(f.Name) != binName {
			continue
		}
		dest := filepath.Join(destDir, binName)
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		out, err := os.Create(dest)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return "", err
		}
		if err := os.Chmod(dest, 0755); err != nil {
			return "", err
		}
		return dest, nil
	}
	return "", fmt.Errorf("zip 中未找到二进制文件")
}

func replaceBinary(newPath, currentPath string) error {
	// Windows 无法直接覆盖运行中的 .exe，先重命名旧文件
	if runtime.GOOS == "windows" {
		oldPath := currentPath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(currentPath, oldPath); err != nil {
			return fmt.Errorf("重命名旧文件失败：%w", err)
		}
	}
	if err := os.Rename(newPath, currentPath); err != nil {
		// Cross-device (tmpDir and execPath on different filesystems): fall back to copy.
		return copyReplace(newPath, currentPath)
	}
	return nil
}

// copyReplace writes src to a sibling temp file then atomically renames it to dst,
// avoiding the cross-device limitation of os.Rename across mount points.
func copyReplace(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开新二进制失败：%w", err)
	}
	defer in.Close()

	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("创建临时文件失败：%w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("写入失败：%w", err)
	}
	out.Close()
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("替换失败：%w", err)
	}
	return nil
}
