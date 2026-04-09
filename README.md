# download_hf

HuggingFace 模型下载工具，支持断点续传、后台运行、国内镜像加速。

## 安装

**macOS / Linux 一键安装：**

```shell
curl -fsSL https://raw.githubusercontent.com/0xYeah/download_hf/main/install.sh | bash
```

**升级：**

```shell
download_hf update
```

安装完成后二进制写入 `~/.download_hf/`，按提示将其加入 PATH 即可全局使用。

> 依赖：`curl`、`unzip`（系统通常已自带）

---

## 功能特性

- 自动获取模型全部文件列表
- 断点续传（中断后重新运行自动继续）
- 实时下载进度条
- 后台运行（nohup 模式，附带日志）
- 可选国内镜像加速（hf-mirror.com）
- 可指定下载根目录

## 用法

```
download_hf [flags] <作者/模型名>
```

### 参数说明

| 参数 | 短参 | 默认值 | 说明 |
|------|------|--------|------|
| `--output` | `-o` | `~/download_models` | 指定下载根目录 |
| `--cn-proxy` | `-p` | 关闭 | 启用国内镜像（hf-mirror.com），默认直连 huggingface.co |
| `--daemon` | `-d` | 关闭 | 后台运行（nohup 模式） |

### 文件保存路径

```
<根目录>/<作者>/<模型名>/
```

默认示例：`~/download_models/Jackrong/Qwopus3.5-27B-v3-GGUF/`

## 示例

```shell
# 直连下载（默认路径）
download_hf Jackrong/Qwopus3.5-27B-v3-GGUF

# 国内镜像加速
download_hf -p Jackrong/Qwopus3.5-27B-v3-GGUF

# 指定保存路径
download_hf -o /data/models Jackrong/Qwopus3.5-27B-v3-GGUF

# 后台下载
download_hf -d Jackrong/Qwopus3.5-27B-v3-GGUF

# 后台+ 国内镜像
download_hf -p -d Jackrong/Qwopus3.5-27B-v3-GGUF

# 组合：国内镜像 + 指定路径 + 后台运行
download_hf -p -o /data/models -d Jackrong/Qwopus3.5-27B-v3-GGUF
```

## 后台运行说明

使用 `--daemon` 时，日志自动写入：

```
~/download_models/logs/download_<作者>_<模型名>.log
```

查看实时日志：

```shell
tail -f ~/download_models/logs/download_Jackrong_Qwopus3.5-27B-v3-GGUF.log
```

## 断点续传

下载中断后，重新执行相同命令即可自动从断点继续，无需额外操作。
