// Package main /main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xYeah/download_hf/actions/update"
	"github.com/cheggaaa/pb/v3"
	"github.com/spf13/cobra"
)

const (
	hfMirror  = "https://hf-mirror.com"
	hfDirect  = "https://huggingface.co"
	baseDirName = "download_models"
)

var mVersion string // injected via ldflags: -X main.mVersion=...

var (
	daemonMode bool
	cnProxy    bool
	outputDir  string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "download_mf [模型仓库ID]",
		Short: "HuggingFace 模型下载工具（支持断点续传+后台运行）",
		Long: `示例：
  download_mf Jackrong/Qwopus3.5-27B-v3-GGUF
  download_mf --daemon Jackrong/Qwopus3.5-27B-v3-GGUF
  download_mf --cn-proxy Jackrong/Qwopus3.5-27B-v3-GGUF
  download_mf --output /data/models Jackrong/Qwopus3.5-27B-v3-GGUF`,
		Args: cobra.ExactArgs(1),
		Run:  runDownload,
	}

	rootCmd.Flags().BoolVarP(&daemonMode, "daemon", "d", false, "后台运行（nohup 模式）")
	rootCmd.Flags().BoolVarP(&cnProxy, "cn-proxy", "p", false, "使用国内镜像（hf-mirror.com），默认直连 huggingface.co")
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

func runDownload(cmd *cobra.Command, args []string) {
	repoID := args[0]

	if daemonMode {
		daemonize(repoID)
		return
	}

	author, modelName, err := parseRepoID(repoID)
	if err != nil {
		fmt.Printf("❌ 解析模型ID失败：%v\n", err)
		os.Exit(1)
	}

	saveDir, err := getSaveDir(author, modelName)
	if err != nil {
		fmt.Printf("❌ 获取保存路径失败：%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("🚀 开始下载模型：%s\n", repoID)
	fmt.Printf("🌐 下载源：%s\n", baseURL())
	fmt.Printf("📂 保存路径：%s\n", saveDir)
	fmt.Println("⏳ 正在获取文件列表...")

	files, err := listModelFiles(repoID)
	if err != nil {
		fmt.Printf("❌ 获取文件列表失败：%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📦 共 %d 个文件需要下载\n", len(files))

	successCount := 0
	failCount := 0
	for i, file := range files {
		fmt.Printf("\n===== 【%d/%d】 下载文件：%s =====\n", i+1, len(files), file)
		if err := downloadFile(repoID, file, saveDir); err != nil {
			fmt.Printf("❌ 下载失败：%v\n", err)
			failCount++
		} else {
			fmt.Printf("✅ 下载完成：%s\n", file)
			successCount++
		}
	}

	fmt.Printf("\n🎉 下载任务完成！成功：%d 个，失败：%d 个\n", successCount, failCount)
	fmt.Printf("📂 模型保存位置：%s\n", saveDir)
}

func parseRepoID(repoID string) (string, string, error) {
	parts := strings.Split(repoID, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("格式错误，必须是 作者/模型名")
	}
	return parts[0], parts[1], nil
}

func getSaveDir(author, modelName string) (string, error) {
	root := outputDir
	if root == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("获取用户目录失败: %w", err)
		}
		root = filepath.Join(homeDir, baseDirName)
	}
	saveDir := filepath.Join(root, author, modelName)
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	return saveDir, nil
}

// hfFileEntry 对应 HF API 返回的文件列表条目
type hfFileEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

func listModelFiles(repoID string) ([]string, error) {
	apiURL := fmt.Sprintf("%s/api/models/%s/tree/main", baseURL(), repoID)
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("请求文件列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 请求失败，状态码：%d", resp.StatusCode)
	}

	var entries []hfFileEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("解析文件列表失败: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.Type == "file" {
			files = append(files, entry.Path)
		}
	}
	return files, nil
}

func downloadFile(repoID, filePath, saveDir string) error {
	fileURL := fmt.Sprintf("%s/%s/resolve/main/%s", baseURL(), repoID, filePath)
	savePath := filepath.Join(saveDir, filePath)

	// 确保子目录存在（模型可能有嵌套目录结构）
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		return fmt.Errorf("创建子目录失败: %w", err)
	}

	var startPos int64
	if info, err := os.Stat(savePath); err == nil {
		startPos = info.Size()
		fmt.Printf("🔁 断点续传：已下载 %d 字节\n", startPos)
	}

	req, err := http.NewRequestWithContext(context.Background(), "GET", fileURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	if startPos > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startPos))
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载失败，状态码：%d", resp.StatusCode)
	}

	flag := os.O_CREATE | os.O_WRONLY
	if startPos > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	file, err := os.OpenFile(savePath, flag, 0644)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	totalSize := startPos + resp.ContentLength
	bar := pb.Full.Start64(totalSize)
	bar.SetCurrent(startPos)
	defer bar.Finish()

	_, err = io.Copy(file, bar.NewProxyReader(resp.Body))
	return err
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

	// 传递除 --daemon 外的所有参数，保留 --cn-proxy 等标志
	var childArgs []string
	for _, arg := range os.Args[1:] {
		if arg != "--daemon" && arg != "-d" && arg != "--daemon=true" {
			childArgs = append(childArgs, arg)
		}
	}

	procAttr := &os.ProcAttr{
		Files: []*os.File{nil, logFd, logFd},
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("❌ 获取可执行文件路径失败：%v\n", err)
		os.Exit(1)
	}

	process, err := os.StartProcess(execPath, append([]string{execPath}, childArgs...), procAttr)
	if err != nil {
		fmt.Printf("❌ 启动后台进程失败：%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 后台下载已启动，PID：%d\n", process.Pid)
	fmt.Printf("📄 日志查看：tail -f %s\n", logFile)
	os.Exit(0)
}
