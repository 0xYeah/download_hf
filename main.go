// Package main /main.go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xYeah/download_hf/actions/download"
	"github.com/0xYeah/download_hf/actions/update"
	"github.com/spf13/cobra"
)

const (
	hfMirror    = "https://hf-mirror.com"
	hfDirect    = "https://huggingface.co"
	baseDirName = "download_models"

	hfTypeModels   = "models"
	hfTypeDatasets = "datasets"
)

var mVersion string // injected via ldflags: -X main.mVersion=...

var (
	daemonMode bool
	cnProxy    bool
	outputDir  string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "download_hf [仓库ID]",
		Short: "HuggingFace 模型/数据集下载工具（多段并发+断点续传+后台运行）",
		Long: `示例：
  download_hf Jackrong/Qwopus3.5-27B-v3-GGUF
  download_hf --daemon Jackrong/Qwopus3.5-27B-v3-GGUF
  download_hf --cn-proxy username/my-dataset
  download_hf --output /data Jackrong/Qwopus3.5-27B-v3-GGUF`,
		Args: cobra.ExactArgs(1),
		Run:  runDownload,
	}

	rootCmd.Flags().BoolVarP(&daemonMode, "daemon", "d", false, "后台运行（nohup 模式）")
	rootCmd.Flags().BoolVarP(&cnProxy, "cn-proxy", "p", false, "使用国内镜像（hf-mirror.com）")
	rootCmd.Flags().StringVarP(&outputDir, "output", "o", "", "指定下载根目录（默认：~/download_models）")
	rootCmd.AddCommand(update.Command(mVersion))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 错误：%v\n", err)
		os.Exit(1)
	}
}

func baseURL() string {
	if cnProxy {
		return hfMirror
	}
	return hfDirect
}

func runDownload(_ *cobra.Command, args []string) {
	repoID := args[0]

	if daemonMode {
		daemonize(repoID)
		return
	}

	author, repoName, err := parseRepoID(repoID)
	if err != nil {
		fmt.Printf("❌ 解析仓库ID失败：%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("🔍 识别仓库类型：%s\n", repoID)
	files, repoType, err := detectAndListFiles(baseURL(), repoID)
	if err != nil {
		fmt.Printf("❌ 获取文件列表失败：%v\n", err)
		os.Exit(1)
	}

	saveDir, err := getSaveDir(repoType, author, repoName)
	if err != nil {
		fmt.Printf("❌ 获取保存路径失败：%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📁 类型：%s\n", repoType)
	fmt.Printf("🌐 下载源：%s\n", baseURL())
	fmt.Printf("📂 保存路径：%s\n", saveDir)
	fmt.Printf("📦 共 %d 个文件\n", len(files))

	successCount, failCount := 0, 0
	for i, file := range files {
		fmt.Printf("\n===== 【%d/%d】 %s =====\n", i+1, len(files), file)
		if err := download.File(baseURL(), repoID, file, saveDir); err != nil {
			fmt.Printf("❌ 下载失败：%v\n", err)
			failCount++
		} else {
			fmt.Printf("✅ 下载完成：%s\n", file)
			successCount++
		}
	}

	fmt.Printf("\n🎉 完成！成功：%d 个，失败：%d 个\n", successCount, failCount)
	fmt.Printf("📂 保存位置：%s\n", saveDir)
}

// detectAndListFiles tries models API first, then datasets API.
// Returns the full recursive file list and the detected repo type.
func detectAndListFiles(base, repoID string) (files []string, repoType string, err error) {
	for _, t := range []string{hfTypeModels, hfTypeDatasets} {
		files, err = listFilesOfType(base, t, repoID, "")
		if err == nil && len(files) > 0 {
			return files, t, nil
		}
	}
	return nil, "", fmt.Errorf("找不到仓库 %s（已尝试 models 和 datasets）", repoID)
}

// hfEntry is a single item returned by the HF tree API.
type hfEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// listFilesOfType recursively lists all files in a HF repo directory.
func listFilesOfType(base, repoType, repoID, subPath string) ([]string, error) {
	apiURL := fmt.Sprintf("%s/api/%s/%s/tree/main", base, repoType, repoID)
	if subPath != "" {
		apiURL += "/" + subPath
	}

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 返回 %d", resp.StatusCode)
	}

	var entries []hfEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		switch e.Type {
		case "file", "blob":
			files = append(files, e.Path)
		case "directory":
			sub, err := listFilesOfType(base, repoType, repoID, e.Path)
			if err != nil {
				return nil, fmt.Errorf("列目录 %s 失败: %w", e.Path, err)
			}
			files = append(files, sub...)
		}
	}
	return files, nil
}

func parseRepoID(repoID string) (string, string, error) {
	parts := strings.Split(repoID, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("格式错误，必须是 作者/仓库名")
	}
	return parts[0], parts[1], nil
}

func getSaveDir(repoType, author, repoName string) (string, error) {
	root := outputDir
	if root == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("获取用户目录失败: %w", err)
		}
		root = filepath.Join(homeDir, baseDirName)
	}
	saveDir := filepath.Join(root, repoType, author, repoName)
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	return saveDir, nil
}

func daemonize(repoID string) {
	homeDir, _ := os.UserHomeDir()
	logDir := filepath.Join(homeDir, baseDirName, "logs")
	os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, fmt.Sprintf("download_%s.log", strings.ReplaceAll(repoID, "/", "_")))

	logFd, err := os.Create(logFile)
	if err != nil {
		fmt.Printf("❌ 创建日志文件失败：%v\n", err)
		os.Exit(1)
	}
	defer logFd.Close()

	var childArgs []string
	for _, arg := range os.Args[1:] {
		if arg != "--daemon" && arg != "-d" && arg != "--daemon=true" {
			childArgs = append(childArgs, arg)
		}
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("❌ 获取可执行文件路径失败：%v\n", err)
		os.Exit(1)
	}

	process, err := os.StartProcess(execPath, append([]string{execPath}, childArgs...), &os.ProcAttr{
		Files: []*os.File{nil, logFd, logFd},
	})
	if err != nil {
		fmt.Printf("❌ 启动后台进程失败：%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 后台下载已启动，PID：%d\n", process.Pid)
	fmt.Printf("📡 子进程参数：%v\n", childArgs)
	fmt.Printf("📄 日志查看：tail -f %s\n", logFile)
	os.Exit(0)
}
