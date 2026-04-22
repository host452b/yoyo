```
 ██    ██  ██████  ██    ██  ██████
  ██  ██  ██    ██  ██  ██  ██    ██
   ████   ██    ██   ████   ██    ██
    ██    ██    ██    ██    ██    ██
    ██     ██████     ██     ██████
```

# yoyo

> [English](README.md) · **简体中文**

**you only yes once** —— 自动批准 AI agent 权限提示的 PTY 代理。

yoyo 坐在你的终端和 AI agent CLI（Claude Code、Codex、Cursor 等）之间，监听 agent 的输出，识别权限提示，在可配置的延迟之后自动发送确认按键——你不再需要盯着屏幕敲 `y`。

---

## 安装

### 一行命令安装（Linux & macOS）

```bash
curl -fsSL https://github.com/host452b/yoyo/releases/latest/download/install.sh | sh
```

脚本自动识别 OS 与架构（linux/darwin × amd64/arm64），不需要本地装 Go。

---

<details>
<summary>手动安装选项</summary>

### 预编译二进制

```bash
# Linux (amd64)
curl -L https://github.com/host452b/yoyo/releases/latest/download/yoyo-linux-amd64 -o yoyo
chmod +x yoyo && sudo mv yoyo /usr/local/bin/

# Linux (arm64)
curl -L https://github.com/host452b/yoyo/releases/latest/download/yoyo-linux-arm64 -o yoyo
chmod +x yoyo && sudo mv yoyo /usr/local/bin/

# macOS (Apple Silicon)
curl -L https://github.com/host452b/yoyo/releases/latest/download/yoyo-darwin-arm64 -o yoyo
chmod +x yoyo && sudo mv yoyo /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/host452b/yoyo/releases/latest/download/yoyo-darwin-amd64 -o yoyo
chmod +x yoyo && sudo mv yoyo /usr/local/bin/
```

### go install（需要 Go 1.21+）

```bash
go install github.com/host452b/yoyo/cmd/yoyo@latest
```

> 先 `go version` 检查版本。Go < 1.21 请用上面的一行命令安装。

### 源码构建

```bash
git clone https://github.com/host452b/yoyo.git
cd yoyo
go build -o yoyo ./cmd/yoyo
sudo mv yoyo /usr/local/bin/
```

</details>

### 验证安装

```bash
yoyo -h
```

---

## 快速开始

```bash
# 默认配置（3 秒延迟后自动 approve）包住 claude
yoyo claude

# 立刻 approve，不等
yoyo -delay 0 claude

# 给 codex 留 5 秒检视窗口
yoyo -delay 5 codex
```

---

## 工作原理

```
┌──────────┐   stdin    ┌───────┐   stdin    ┌───────────┐
│  终端    │ ─────────► │ yoyo  │ ─────────► │ AI agent  │
│          │ ◄───────── │ 代理  │ ◄───────── │  (PTY)    │
└──────────┘  stdout    └───────┘  stdout    └───────────┘
                            │
                   检测到 prompt?
                   等 delay 秒
                   发 "yes" 按键
```

1. yoyo 在 PTY（伪终端）里起 agent，让 agent 以为有真人在敲键。
2. 每一帧 PTY 输出经过 VT100 屏幕缓冲，解出可见文本。
3. 文本依次过规则链（自定义规则 → 内置 detector）。
4. 命中后，yoyo 等 `delay` 秒，然后发送 approval 按键。
5. 倒计时期间按任意"真"按键就能取消自动批准。
6. 终端右下角的状态条实时显示当前状态。

---

## 状态条

```
[yoyo: on 3s]           已启用，3 秒延迟，当前无 prompt
[yoyo: on 2s | Claude]  检测到 prompt，倒计时中（剩 2 秒）
[yoyo: on 0s | seen: X] 本会话已批准过同一 prompt，立即发送
[yoyo: off]             已关闭自动批准（手动模式）
[yoyo: ^Y …]            正在等 Ctrl+Y 后面的命令键
[yoyo: dry 3s]          dry-run 模式——只检测不批准
```

- **绿色** = 自动批准生效
- **黄色** = 倒计时中 / dry-run / 等待 Ctrl+Y 命令
- **红色** = 自动批准已关闭

---

## 支持的 Agent

| Agent | 命令 | 检测方式 |
|-------|------|---------|
| Claude Code | `claude` | `───` 包围的权限框 + Yes/No 选项 |
| OpenAI Codex | `codex` | "Would you like to" / "needs your approval" + "Press enter to confirm" |
| Cursor | `cursor`、`cursor-agent` | `┌─┐` 画框 + `(y)` / `n)` 选项 |
| 未知 | 任意命令 | 三个 detector 并行跑；前 10 帧内根据屏幕内容自动识别 agent |

---

## CLI 参考

```
yoyo [flags] <command> [args...]
```

### Flag

| Flag | 默认 | 说明 |
|------|------|------|
| `-delay int` | `3`（来自 config） | 自动批准前等待的秒数。`0` = 立即；`-1` = 读取 config 值。显式传入总是优先于 per-agent 配置。 |
| `-config string` | `~/.config/yoyo/config.toml` | TOML 配置文件路径，支持 `~/`。 |
| `-log string` | `~/.yoyo/yoyo.log` | 日志文件路径，支持 `~/`。 |
| `-dry-run` | off | 只检测不发送 approval 按键。状态条显示 `dry` 而不是 `on`，用来测试自定义规则。 |
| `-v` | | 打印版本后退出。 |

`yoyo -h` 查看完整帮助。

### 运行时控制

前缀键是 **Ctrl+Y**。先按 Ctrl+Y，再按：

| 键 | 动作 |
|-----|------|
| `0` | 切换自动批准 开/关 |
| `1`–`5` | 把延迟设为 N 秒（如果之前是关也会重新开启） |
| `a` | 切换 AFK 模式 开/关（与自动批准独立） |
| `f` | 切换 fuzzy 保底检测 开/关 |

**取消当前倒计时**：倒计时期间按任意非 escape 键即可。

### AFK 模式

有些 agent 的 prompt 不在 yoyo 的 detector 词表里，agent 会永远卡在 `read()` 等输入。`-afk` 是一个"无脑"空闲定时器，在静默一段时间后去戳 agent：

```
yoyo -afk -afk-idle 10m claude
```

只要终端在配置的窗口内**既没输出也没输入**，yoyo 就先发 `y` + Enter，短暂停顿，再发 `continue, Choose based on your project understanding.` + Enter，然后重新起计时窗口。运行时用 `Ctrl+Y a` 切换开关。

### 删除命令安全防护

默认**开启**。当当前屏幕内容包含**删除/清理类**命令时，yoyo 拒绝自动批准。覆盖范围：

- `rm -rf /`、`rm -rf ~`、`rm -rf *`（顶层或通配，不影响作用域明确的路径）
- `git rm -r`、`git clean -f…`
- `find … -delete`、`find … -exec rm`
- SQL 的 `DROP DATABASE/TABLE/SCHEMA/USER`、`TRUNCATE TABLE`、裸 `DELETE FROM`（无 WHERE，尽力识别）
- `kubectl delete <任何资源>`
- `terraform destroy` / `terraform apply -destroy`
- `docker` / `podman volume rm`、`system prune`、`image prune -a`

命中时状态条变成 `danger: <匹配片段>`，日志也会记原因。你仍然可以手动按 `y` / Enter 自己批准。

若你在受限环境（一次性容器、脚本化清理循环等）里跑、不在乎 `rm -rf`，可用 `-no-safety` 关闭。该防护刻意保持窄——**不会**拦 `mkfs`、`dd`、`chmod`、`chown`、`curl | sh`、`git push --force` 或 fork bomb 这类（容器环境下你常用，不该被挡）。

**配置文件权限**启动时也会检查。如果 `config.toml` 可被 group 或 other 写入，stderr 会警告——可写配置意味着攻击者可以注入 `[[rules]]` 批准任何东西。加固方法：`chmod 600 ~/.config/yoyo/config.toml`。

### Fuzzy 保底

第二层可选 detector，兜住内置 detector 认不出来的 y/n prompt。同时满足两个条件才触发：（1）屏幕在 `-fuzzy-stable`（默认 3 秒）窗口内保持不变；（2）最后 15 行里出现精确词表中的 y/n 标记，如 `(y/n)`、`[Y/n]`、`y/n?`、`yes/no` 等。光是 `Yes`、`enter` 这类常见英文词**不会**触发（防止日志、代码片段里的误报）。

```
yoyo -fuzzy claude
```

Fuzzy 命中后走标准 approval 流程，所以 `-delay` 和 memory 去重照常起效。运行时用 `Ctrl+Y f` 切换。推荐和 `-afk` 同时开：fuzzy 在几秒内兜住可识别的 y/n 卡死，AFK 在更长的 idle 窗口之后兜住其他所有情况。

---

## 配置文件

默认路径：`~/.config/yoyo/config.toml`

```toml
[defaults]
delay        = 3              # 批准延迟秒数（0 = 立即）
enabled      = true           # 启动时就开自动批准
afk          = false          # 启用 AFK 空闲戳
afk_idle     = "10m"          # AFK idle 阈值
fuzzy        = false           # 启用通用 fuzzy 保底
fuzzy_stable = "3s"            # fuzzy 触发前屏幕稳定窗口
log_file     = "~/.yoyo/yoyo.log"

# 按 agent 覆盖延迟（显式 -delay flag 会覆盖这里）
[agents.claude]
delay = 0                     # claude 的 prompt 立刻批准

[agents.codex]
delay = 5                     # 给 codex 的 prompt 5 秒检视时间

# 全局自定义规则——优先于内置 detector，按顺序命中
[[rules]]
name     = "my-tool-confirm"
pattern  = "Continue\\? \\[y/N\\]"   # Go regexp，匹配整屏可见文本
response = "y\r"                      # 命中后发给 agent 的按键

# 指定 agent 的自定义规则——优先于该 agent 的全局规则
[[agents.claude.rules]]
name     = "claude-custom"
pattern  = "Are you sure you want to"
response = "y\r"
```

### 规则字段

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | 否 | 命中时状态条显示的标签（默认 `"custom"`） |
| `pattern` | 是 | Go 正则，匹配整屏可见文本 |
| `response` | 是 | 发给 agent 的按键。`\r` = Enter、`\t` = Tab，以此类推。 |

### 规则链顺序

对某个 agent，规则按此优先级评估，第一个命中就生效：

1. 该 agent 的自定义规则（`agents.<name>.rules`）
2. 全局自定义规则（`rules`）
3. 该 agent 的内置 detector
4. Fuzzy 保底（如果 `-fuzzy` 开启且屏幕稳定窗口满足）

---

## 会话记忆

yoyo 会记住当前会话中批准过的每一个 prompt（用 SHA-256 哈希 cleaned body 做 key）。同一 prompt 再次出现时，无视 `-delay` 直接批准，状态条显示 `seen: <rule>`。记忆只在当前进程内有效，yoyo 退出即清空。

---

## 退出行为

- 子进程退出时 yoyo 跟着退出。
- `SIGINT`、`SIGTERM`、`SIGHUP`、`SIGQUIT` 都会恢复终端然后干净退出。
- 即使 yoyo 内部 panic，也通过 recover 保证终端被恢复。
- 退出时 stderr 会打印汇总：`yoyo: 42 prompt(s) auto-approved`。

---

## 日志

yoyo 写日志到 `~/.yoyo/yoyo.log`（可用 `-log` 或 config 里的 `defaults.log_file` 改路径）。

```
[INFO]  2026-04-02 14:32:15.123 started claude (kind=claude, delay=3s)
[INFO]  2026-04-02 14:32:20.456 prompt detected: Claude
[INFO]  2026-04-02 14:32:23.456 approval timer fired, sending response for: Claude
[ERROR] 2026-04-02 14:32:24.789 vt10x panic recovered: index out of range
```

实时跟日志：

```bash
tail -f ~/.yoyo/yoyo.log
```

---

## 常见问题

**我的 prompt 没被识别出来**

1. 加 `-dry-run` 看看 yoyo 是否能识别（但不会真发 approval）：
   ```bash
   yoyo -dry-run my-agent
   ```
   状态条能显示规则名说明检测正常，问题可能在 delay 或 enabled 配置上。

2. 盯日志看检测事件：
   ```bash
   tail -f ~/.yoyo/yoyo.log
   ```

3. 自定义 agent 可以在 config 里加 `[[rules]]`，正则匹配屏幕上的可见文本（不是带 ANSI 转义的原始字节）。

4. 打不通可以试试 `-fuzzy` 保底，或者 `-afk` 空闲兜底。

**yoyo 退出后终端是坏的**

跑 `reset` 恢复终端。这种情况一般是 yoyo 被 `SIGKILL`（无法捕获），panic recovery 才没机会还原终端。

**状态条闪烁或根本不显示**

- 确认终端支持 ANSI 转义（绝大多数支持）。
- 终端太窄（< 24 列）时状态条会自动隐藏。
- 有 SIGWINCH 监听，终端缩放时状态条会重定位。

---

## 平台支持

| 平台 | 状态 |
|------|------|
| Linux | 完全支持 |
| macOS | 完全支持 |
| Windows | 能编译能运行；PTY resize 是 no-op |

源码构建需要 Go 1.21+。

---

## 安全

### Prompt 注入

yoyo 会自动批准匹配上 detector 规则的 prompt。如果 agent 处理的程序或文件恶意输出一段看起来像权限提示的文本，yoyo 可能会批准你本意没打算批准的操作。

**缓解措施：**

- 用 `-delay` 留一点检视时间。默认 3 秒，倒计时内按任意键都能取消。
- agent 要处理不可信输入之前，`Ctrl+Y 0` 关掉 yoyo。
- config 里的 `pattern` 规则越严越好——越宽泛注入面越大。

yoyo 是为你信任 agent 及其环境的开发工作流设计的，不是为对抗性环境设计的。

---

## 变更日志

参见 [CHANGELOG.md](CHANGELOG.md)。

---

## 许可

MIT
