# net-notify

周期性探测公网 URL 的连通性。**无配置文件且命令行未指定 `-url`** 时，内置两大站点的默认分组：**Google + GitHub** 一组（`all_fail`，两者都失败才告警该组）、**百度**一组（`any_fail`）；亦可在 JSON 中用 **`groups`** 自定义。**多组并存时任一组满足自身策略即视为本轮告警**。仅写 **`urls`** 的 flat 配置表示整表任一失败告警。HTTP 请求带 **no-cache** 相关头，不启用磁盘缓存。

默认 **通知后端为 `notify-send`**（`libnotify`，需在本机图形会话中可用）。**严重程度（urgency）默认 `critical`**，经 **`notify-send -u critical`** 发往 Freedesktop 通知服务。**若将 `notify_backend` 设为 `dms`**（DankMaterialShell）且 **`notify_urgency` 为 `normal`**，则走 **`dms notify`**；其它 urgency 下即使用 `dms` 后端仍会 **回退到 `notify-send -u`**（因 `dms notify` 无法单独设紧急度）。

## 依赖

- **`notify-send`**（`libnotify`，**默认后端**）：桌面通知；与 DMS Material shell 等共用 DBus 管线时也可显示。
- **`dms`**（DankMaterialShell，**可选**）：仅当 **`notify_backend`** 为 **`dms`** 且 **`notify_urgency` 为 `normal`** 时必须；用于走 `dms notify`。

## 构建

```bash
go build -o net-notify ./cmd/net-notify
```

或从 GitHub Release 下载 **Linux** 预编译包（**amd64**、**arm64** 的 `.tar.gz`，及 `checksums.txt`）。

## 用法

```bash
# 持续运行（默认每 1 分钟探测；内置分组：谷歌+GitHub 需全失败、百度任一失败）
./net-notify run

# 自定义间隔与 URL（整表视为一组，策略为任一失败）
./net-notify run -interval 30s -url https://example.com

# 同上，但仅在列表内全部 URL 都失败时才告警（仅与 -url 联用）
./net-notify run -url https://a -url https://b -notify-when all-fail

# 仅跑一轮；失败时仍发通知（不受冷却限制），退出码非 0
./net-notify run -once

# 单次检查，打印报告，不发通知（与 run 相同的分组 / notify_when 语义；便于脚本）
./net-notify check
```

配置文件为 **JSON**（见 [packaging/config.example.json](packaging/config.example.json)）。使用 `-config` 指定；命令行 flag 会覆盖文件中对应项（命令行若出现 `-url`，则**仅使用**命令行 URL，并忽略配置文件里的 **`urls` 与 `groups`**）。配置项 `verbose: true` 或 `run -verbose` 会在每轮探测后向 stderr 打一行摘要，便于 `journalctl --user -u net-notify` 对照间隔（默认 `interval` 为 **1m**）。**`notify_urgency`**：`low` | `normal` | `critical`（默认 `critical`）。

### 分组与 `notify_when`（内置默认与 JSON）

不能与顶层 **`urls`** 同时出现：`groups` 非空时请删去（或不要写）`urls`。每个分组：`name`、`urls`、`notify_when` 为 **`any_fail`**（组内任一失败即该组告警）或 **`all_fail`**（组内全部失败该组才告警）。同一 URL 可出现在多个组中；探测会去重，`all_fail` 常用于「互为备份的一组站点，单个失败不响、全挂才响」。

[packaging/config.example.json](packaging/config.example.json) 与内置默认分组一致：**Google+GitHub**（`all_fail`）与 **百度**（`any_fail`）。

以下为等价的自建配置示例片段：

```json
{
  "interval": "1m",
  "timeout": "10s",
  "groups": [
    {
      "name": "Google+GitHub",
      "urls": [
        "https://www.google.com",
        "https://www.github.com"
      ],
      "notify_when": "all_fail"
    },
    {
      "name": "百度",
      "urls": ["https://www.baidu.com"],
      "notify_when": "any_fail"
    }
  ],
  "alert_cooldown": "15m",
  "notify_timeout_ms": 30000,
  "notify_icon": "network-error",
  "notify_app": "net-notify",
  "notify_backend": "notify-send",
  "dms_path": "",
  "verbose": false,
  "notify_urgency": "critical"
}
```

仅用顶层 **`urls`**、不做分组的写法仍受支持（整表 `any_fail`）。

**说明**：程序**只在探测失败时**推送通知；网络一直正常时不会弹窗（除你手动执行的 `test-notify`）。经 DBus / 通知服务器发送时，**过长或特殊标题**可能触发实现方错误；默认失败标题已缩短为 **`网络探测失败`**。

### 告警冷却

持续失败时，默认 **15 分钟内最多重复通知一次**（`-alert-cooldown`）。从正常变为失败时**立即**通知。

## systemd（用户服务）

建议在登录会话中以 **user unit** 运行，以便桌面通知可用：

```bash
install -D -m755 net-notify ~/.local/bin/net-notify
mkdir -p ~/.config/net-notify
cp packaging/config.example.json ~/.config/net-notify/config.json
# 按需编辑 config.json
cp packaging/net-notify.service.example ~/.config/systemd/user/net-notify.service
# 编辑 ExecStart 中的二进制路径与配置文件路径
systemctl --user daemon-reload
systemctl --user enable --now net-notify.service
```

如需在无图形会话下排障，可使用 `net-notify check`；长时间离席可配合 `loginctl enable-linger $USER`（按需）。

## 开发

```bash
go test ./...
go vet ./...
```

CI：`push`/`pull_request` 至 `main`（或 `master`）时在 **linux/amd64**（`ubuntu-latest`）与 **linux/arm64**（`ubuntu-24.04-arm`）上跑同一套命令。

Release 工作流由 [GoReleaser](https://goreleaser.com/) 打包，**仅提供 Linux `amd64` 与 `arm64`**（`net-notify_<version>_linux_<arch>.tar.gz` 及 `checksums.txt`）。推送 semver 标签即触发：

```bash
git tag v0.3.0
git push origin v0.3.0
```

将 `go.mod` 中的 `module` 改为你的 `github.com/<用户>/net-notify` 后再推送，以便 `go install` 路径一致。

MIT
