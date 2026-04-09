package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "升级到最新版本",
	Run:   runUpdate,
}

type ghRelease struct {
	TagName string `json:"tag_name"`
}

func runUpdate(_ *cobra.Command, _ []string) {
	latest, err := fetchLatestVersion()
	if err != nil {
		fmt.Printf("❌ 获取最新版本失败：%v\n", err)
		os.Exit(1)
	}

	if mVersion != "" && mVersion == latest {
		fmt.Printf("✅ 已是最新版本 %s\n", mVersion)
		return
	}

	if mVersion != "" {
		fmt.Printf("🔄 升级 %s -> %s\n", mVersion, latest)
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
	resp, err := http.Get("https://api.github.com/repos/0xYeah/download_hf/releases/latest")
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
	return os.Rename(newPath, currentPath)
}
