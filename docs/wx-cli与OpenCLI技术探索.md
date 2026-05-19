# wx-cli 与 OpenCLI 技术探索记录

日期：2026-05-19  
环境：Windows / PowerShell / Node.js v22.19.0 / npm 10.9.3  
工作目录：`C:\data\liang\code\github_projects\we-flow`

## 背景

本次探索目标是实操验证两个工具是否能在当前机器上跑起来，并判断它们是否适合作为后续 `we-flow` 集成微信本地数据和通用 CLI adapter 能力的基础。

涉及工具：

- `wx-cli`：微信本地数据 CLI，用于读取本机微信数据库，支持会话、聊天记录、搜索、联系人、导出等能力。
- `OpenCLI`：把网站、浏览器、外部 CLI 包装成统一 CLI 的工具，内置大量站点 adapter，也声明支持 `wx(wx-cli)` 作为 external CLI。

参考链接：

- https://github.com/jackwener/wx-cli
- https://opencli.info/docs/guide/getting-started.html

## 安装与版本

本机一开始没有安装 `wx` 和 `opencli`：

```powershell
where.exe wx
where.exe opencli
```

均未找到。

通过 npm registry 查询到：

```powershell
npm view @jackwener/wx-cli version bin dist.tarball
npm view @jackwener/opencli version bin dist.tarball
```

结果：

- `@jackwener/wx-cli@0.3.0`
  - bin：`wx -> bin/wx.js`
- `@jackwener/opencli@1.7.22`
  - bin：`opencli -> dist/src/main.js`

后续按官方推荐方式做了全局安装：

```powershell
npm install -g @jackwener/wx-cli @jackwener/opencli
```

安装后验证：

```powershell
wx --version
opencli --version
where.exe wx
where.exe opencli
```

结果：

- `wx 0.3.0`
- `opencli 1.7.22`
- `wx` 和 `opencli` 均位于 `C:\Users\liang\AppData\Roaming\npm`

## wx-cli 实测结果

### 帮助命令

```powershell
npx -y @jackwener/wx-cli --help
```

命令成功。可用能力包括：

- `init`：初始化，检测微信数据目录并扫描加密密钥
- `sessions`：列出最近会话
- `history`：查看聊天记录
- `search`：搜索消息
- `contacts`：查看联系人
- `export`：导出聊天记录
- `unread`：显示未读消息会话
- `members`：查看群成员
- `new-messages`：获取自上次检查以来的新消息
- `stats`：聊天统计分析
- `favorites`：微信收藏内容
- `sns-*`：朋友圈相关本地缓存查询
- `biz-articles`：公众号文章推送查询
- `attachments` / `extract`：附件列出与提取
- `daemon`：管理 `wx-daemon`

### 初始化

```powershell
npx -y @jackwener/wx-cli init
```

初始化成功，关键信息：

- 找到了微信数据目录：
  - `C:\Users\liang\Documents\xwechat_files\liangz77_f5d2\db_storage`
- 成功提取 18 个数据库密钥
- 写入配置：
  - `C:\Users\liang\.wx-cli\all_keys.json`
  - `C:\Users\liang\.wx-cli\config.json`
- 检测到微信进程
- 匹配到数据库密钥

注意：这一步会读取本机微信进程和本地数据库，涉及隐私数据。后续集成时应避免把原始聊天内容、联系人信息、密钥文件写入仓库或日志。

### 只读命令

验证 `sessions`：

```powershell
npx -y @jackwener/wx-cli sessions -n 3 --json
```

结果：

- 退出码为 0
- 返回 JSON 对象
- 顶层字段：`meta,sessions`
- `sessions` 数量为 3
- `meta` 中包含：
  - `chat_latest_db`
  - `chat_latest_timestamp`
  - `shards_hit`
  - `shards_scanned`
  - `status`
  - `unknown_shards`

验证 `unread`：

```powershell
npx -y @jackwener/wx-cli unread -n 3 --json
```

结果：

- 退出码为 0
- 返回 JSON 对象
- 顶层字段：`meta,sessions,total`
- `sessions` 数量为 3
- `total=3`

验证 `contacts`：

```powershell
npx -y @jackwener/wx-cli contacts -n 3 --json
```

结果：

- 退出码为 0
- 返回 `[]`

当前记录没有展开任何具体会话、联系人或消息内容，仅保留结构性验证结果。

### daemon

```powershell
wx daemon status
```

结果显示 `wx-daemon` 正在运行。

### wx-cli 结论

`wx-cli` 在当前 Windows 环境中可用，至少以下路径已验证成功：

- 安装
- 初始化
- 密钥扫描
- 会话读取
- 未读会话读取
- 联系人命令执行
- daemon 状态查看

后续若要在 `we-flow` 中接入微信本地数据，建议优先直接调用 `wx` 命令，而不是先经过 OpenCLI 的 external CLI bridge。

## OpenCLI 实测结果

### 帮助命令

```powershell
npx -y @jackwener/opencli --help
```

命令成功。OpenCLI 识别到：

- 12 个 external CLIs
- 7 个 app adapters
- 136 个 site adapters

其中 external CLIs 包含：

- `wx(wx-cli)`
- `docker`
- `gh`
- `vercel`
- `wecom-cli`
- `tg`
- `discord`
- 等

### 站点 adapter 验证

验证 `wikipedia` adapter 的命令结构：

```powershell
npx -y @jackwener/opencli wikipedia --help -f yaml
```

成功返回结构化 YAML，包含：

- `page`
- `random`
- `search`
- `summary`
- `trending`

实际访问 Wikipedia：

```powershell
npx -y @jackwener/opencli wikipedia search "OpenAI" --limit 3 --lang en -f json
```

失败：

```text
Connect Timeout Error (attempted address: en.wikipedia.org:443, timeout: 10000ms)
```

判断：命令结构正常，失败更像当前网络到 `en.wikipedia.org` 的连通性问题，不是 OpenCLI 本体启动失败。

验证 `npm` adapter：

```powershell
npx -y @jackwener/opencli npm search opencli --limit 3 -f json
```

成功，返回真实 npm 查询结果，包括 `@jackwener/opencli@1.7.22`。

验证 `npm` adapter 定义：

```powershell
opencli validate npm
```

成功：

```text
opencli validate: PASS
Checked 3 command(s)
Errors: 0  Warnings: 0
```

验证 smoke test：

```powershell
opencli verify npm --smoke
```

结果：

- validate 通过
- smoke test 失败但原因是当前包/环境没有可用 smoke tests

```text
Smoke: FAIL (skipped) — Smoke tests are unavailable in this package/environment.
```

这不代表 `npm` adapter 不可用。

### 浏览器 bridge 验证

测试浏览器型 adapter：

```powershell
opencli bilibili hot --limit 5 -f json
```

失败：

```text
BROWSER_CONNECT
Browser Bridge extension not connected
```

运行：

```powershell
opencli doctor -v
```

结果：

- OpenCLI daemon 正常运行
- Browser Bridge extension 未连接
- 浏览器连通性失败

```text
[OK] Daemon: running on port 19825 (v1.7.22)
[MISSING] Extension: not connected
[FAIL] Connectivity: failed (Browser Bridge extension not connected)
```

OpenCLI 文档提示需要安装浏览器扩展：

1. 下载 OpenCLI releases 中的扩展
2. 打开 `chrome://extensions`
3. 开启 Developer Mode
4. Load unpacked

当前尚未完成这一步，因此浏览器型 adapter 还不能使用。

### external CLI：wx 集成问题

OpenCLI 能识别 `wx` 已安装：

```powershell
opencli external list -f json
```

其中 `wx` 显示：

```json
{
  "name": "wx",
  "package": "wx-cli",
  "binary": "wx",
  "installed": true
}
```

但实际执行：

```powershell
opencli wx --help -f yaml
```

失败：

```text
Failed to execute 'wx': spawnSync wx ENOENT
```

确认 `wx` 在 PATH 中：

```powershell
where.exe wx
```

结果：

```text
C:\Users\liang\AppData\Roaming\npm\wx
C:\Users\liang\AppData\Roaming\npm\wx.cmd
```

尝试手动注册 Windows npm shim：

```powershell
opencli external register wx-win --binary wx.cmd --desc "WeChat CLI via Windows npm cmd shim"
```

再执行：

```powershell
opencli wx-win --help -f yaml
```

失败：

```text
Failed to execute 'wx.cmd': spawnSync wx.cmd EINVAL
```

尝试注册绝对路径：

```powershell
opencli external register wx-cmd --binary "C:\Users\liang\AppData\Roaming\npm\wx.cmd" --desc "WeChat CLI via absolute Windows npm cmd shim"
```

执行时被 OpenCLI 判断为未安装，并要求手动安装。

实验注册项已清理：

```powershell
Remove-Item -LiteralPath C:\Users\liang\.opencli\external-clis.yaml
```

### OpenCLI daemon

```powershell
opencli daemon status
```

结果：

- daemon 正常运行
- 版本：`v1.7.22`
- 端口：`19825`
- extension：`disconnected`

### OpenCLI 结论

OpenCLI 在当前环境中具备以下可用能力：

- 安装成功
- 命令入口可用
- 内置 adapter 列表可用
- `npm` 这类非浏览器 adapter 可用
- adapter validate 可用
- daemon 可运行

当前限制：

- 部分站点因网络连通性失败，例如 Wikipedia、Hacker News。
- 浏览器型 adapter 依赖 Browser Bridge 扩展，当前未安装或未连接。
- `opencli wx` external CLI 集成在 Windows 上无法正常 spawn `wx` / `wx.cmd`，疑似 OpenCLI 对 Windows npm shim 的兼容性问题。

## 对 we-flow 的接入建议

### 短期建议

短期接入微信本地数据时，建议直接调用 `wx-cli`：

```powershell
wx sessions --json
wx unread --json
wx history <CHAT> --json
wx search <QUERY> --json
```

理由：

- `wx-cli` 本体已验证可用。
- `opencli wx` 当前在 Windows 上无法执行。
- 直接调用 `wx` 能减少一层不稳定 bridge。

### OpenCLI 的适用边界

OpenCLI 可以作为后续通用 adapter 层继续探索，尤其适合：

- npm、pypi、github、wikipedia 等站点查询类 adapter
- 未来安装 Browser Bridge 后的浏览器自动化 adapter
- 将多个外部工具统一成 agent 友好的 CLI 接口

但当前不要把 `opencli wx` 作为微信能力的主路径。

### 隐私与安全

`wx-cli` 会读取本机微信数据库和密钥配置。后续工程化时需要注意：

- 不提交 `C:\Users\liang\.wx-cli\all_keys.json`
- 不提交 `C:\Users\liang\.wx-cli\config.json`
- 不把原始聊天内容写入普通日志
- 对导出内容做最小化、脱敏或用户显式授权
- 如果做缓存，明确缓存位置和清理策略

### 后续待办

1. 在 `we-flow` 中封装一个薄的 `wx` 调用层，优先支持 `sessions`、`unread`、`history`、`search`。
2. 统一解析 `wx --json` 输出，避免依赖 YAML 文本格式。
3. 对微信数据读取加权限提示和隐私边界。
4. 如果需要 OpenCLI 浏览器能力，安装并验证 Browser Bridge 扩展。
5. 给 OpenCLI 提一个 Windows external CLI bridge 的 issue 或补丁，重点描述 `spawnSync wx ENOENT` 和 `spawnSync wx.cmd EINVAL`。

## 最终判断

`wx-cli`：当前机器上可实用，适合作为微信本地数据读取的第一阶段技术方案。  
`OpenCLI`：本体可用，部分 adapter 可用，但浏览器 adapter 和 `wx` external CLI bridge 仍需额外配置或修复；暂时适合作为辅助探索工具，不建议作为微信接入的主路径。
