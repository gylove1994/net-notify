# net-notify

周期性探测公网 URL 的连通性；**任一**目标失败时通过 **DMS**（`dms notify`，DankMaterialShell）或 **`notify-send -u critical`** 推送桌面通知。HTTP 请求带 **no-cache** 相关头，不启用磁盘缓存。

## 依赖

- 若使用默认后端：已安装并在 `PATH` 中的 **`dms`**（DankMaterialShell）。
- 可选：`notify-send`（`libnotify`），通过 `-notify-backend notify-send` 使用最高紧急级桌面通知。

## 构建

```bash
go build -o net-notify ./cmd/net-notify
```

或从发布页下载预编译二进制。

## 用法

```bash
# 持续运行（默认每 1 分钟探测 Google / GitHub / 百度）
./net-notify run

# 自定义间隔与 URL
./net-notify run -interval 30s -url https://example.com

# 仅跑一轮；失败时仍发通知（不受冷却限制），退出码非 0
./net-notify run -once

# 单次检查，打印报告，不发通知（便于脚本 / CI）
./net-notify check
```

配置文件为 **JSON**（见 [packaging/config.example.json](packaging/config.example.json)）。使用 `-config` 指定；命令行 flag 会覆盖文件中对应项（URL 若在命令行出现 `-url`，则**仅使用**命令行 URL）。配置项 `verbose: true` 或 `run -verbose` 会在每轮探测后向 stderr 打一行摘要，便于 `journalctl --user -u net-notify` 对照间隔（默认 `interval` 为 **1m**）。

**说明**：程序**只在探测失败时**推送通知；网络一直正常时不会弹窗（除你手动执行的 `test-notify`）。DMS 经 DBus 发送通知时，**过长/过长的纯中文标题**可能触发其内部错误；默认失败标题已缩短为 **`网络探测失败`**。

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

## 发版（维护者）

推送 semver 标签触发 [GoReleaser](https://goreleaser.com/) 工作流，生成 GitHub Release 与校验文件：

```bash
git tag v0.1.0
git push origin v0.1.0
```

将 `go.mod` 中的 `module` 改为你的 `github.com/<用户>/net-notify` 后再推送，以便 `go install` 路径一致。

## 许可证

MIT
