# wx-cli 字段结构验证报告

日期：2026-05-19

## 验证目标

围绕“实时获取本机微信聊天记录，并交给 AI 分析”的需求，验证 `wx-cli` 是否能提供第一阶段 MVP 所需的数据结构。

重点验证：

- 最近会话结构
- 增量新消息结构
- 群聊历史记录结构
- 群成员结构
- 私聊历史记录可用性
- 消息类型过滤
- 去重字段
- 实时性测试前置条件

## 1. sessions 验证

命令：

```powershell
wx sessions -n 10 --json
```

结果：成功。

顶层字段：

- `meta`
- `sessions`

单条 session 字段：

- `chat`
- `chat_type`
- `is_group`
- `last_msg_type`
- `last_sender`
- `summary`
- `time`
- `timestamp`
- `unread`
- `username`

结论：

- 可以区分会话类型。
- 可以区分群聊和非群聊。
- 可以拿到最后消息类型、最后发送人、摘要、时间、未读数。
- `chat_type` 已观察到的值包括：
  - `group`
  - `private`
  - `official_account`
  - `folded`

## 2. new-messages 验证

命令：

```powershell
wx new-messages -n 10 --json
```

结果：成功。

顶层字段：

- `count`
- `messages`
- `meta`
- `new_state`

单条 message 字段：

- `chat`
- `chat_type`
- `content`
- `is_group`
- `sender`
- `time`
- `timestamp`
- `type`
- `username`
- 部分链接消息存在 `url`

结论：

- `new-messages` 可以返回群聊消息。
- 消息里有会话、会话类型、发送人、时间、时间戳、类型、内容。
- 群聊消息中 `sender` 可用于识别具体发送者。
- `new_state` 是一个按会话记录最新时间戳的状态表。
- `new-messages` 没有返回 `local_id`，即使用 `--with-meta` 也没有。

## 3. 群聊 history 验证

命令：

```powershell
wx history "56042420718@chatroom" -n 5 --json
```

结果：成功。

顶层字段：

- `chat`
- `chat_type`
- `count`
- `is_group`
- `messages`
- `meta`
- `username`

单条 message 字段：

- `content`
- `local_id`
- `sender`
- `time`
- `timestamp`
- `type`
- 部分链接消息存在 `url`

结论：

- 群聊历史记录可以读取。
- 群聊历史记录有 `local_id`。
- `local_id + chat/username` 可以作为较好的本地去重 key。
- `sender` 可与群成员列表中的 `username` 对应。

## 4. 群成员 members 验证

命令：

```powershell
wx members "56042420718@chatroom" --json
```

结果：成功。

单个 member 字段：

- `contact_display`
- `display`
- `group_nickname`
- `is_owner`
- `username`

结论：

- 群成员可以读取。
- 群成员的 `username` 可以和 `history` 里的 `sender` 对应。
- 可以判断群主：`is_owner=true`。
- 可用显示名包括联系人显示名、群昵称、展示名。

注意：

- 本次观察到部分成员的 `display/group_nickname` 字段异常重复，后续产品里需要优先使用 `contact_display`，并保留 fallback。

## 5. 私聊 history 验证

命令：

```powershell
wx history "<private_chat>" -n 2 --json
```

验证方式：

- 从 `wx sessions -n 200 --json` 中筛选 `chat_type=private` 的会话。
- 逐个使用 `chat` 调用 `wx history`。

结果：

- 共发现 20 个 private 候选。
- 多数候选无法直接读取 history，错误包括：
  - `找不到联系人`
  - `找不到 <chat> 的消息记录`
- 第 18 个候选读取成功。

成功返回字段：

- `chat`
- `chat_type`
- `count`
- `is_group`
- `messages`
- `meta`
- `username`

私聊 message 字段：

- `content`
- `local_id`
- `sender`
- `time`
- `timestamp`
- `type`

结论：

- 私聊历史记录技术上可读取。
- 但不是所有 `sessions` 中的 private 会话都能直接用 `chat` 作为 `history` 参数。
- 私聊匹配规则需要继续研究。
- 第一版不应假设 `sessions[].chat` 对所有 private 会话都可直接读取 history。

## 6. 消息类型过滤验证

### 文本

命令：

```powershell
wx history "56042420718@chatroom" --type text -n 3 --json
```

结果：成功。

### 图片

命令：

```powershell
wx history "56042420718@chatroom" --type image -n 3 --json
```

结果：成功。

图片内容格式示例：

```text
[图片] local_id=<id>
```

### 语音

命令：

```powershell
wx history "3378730735@chatroom" --type voice -n 3 --json
```

结果：成功。

语音内容格式：

```text
[语音]
```

### 链接/文件

命令：

```powershell
wx history "56042420718@chatroom" --type file -n 3 --json
```

结果：成功。

部分链接/文件消息存在：

- `url`

结论：

- 第一版可以稳定支持文本消息。
- 链接消息可以读取标题式内容，部分可以读取 URL。
- 图片、语音、文件可以作为事件记录，但暂时不能直接获得图片 OCR 或语音转文字内容。

## 7. 去重字段判断

### history

`history` 有：

- `chat`
- `username`
- `local_id`
- `timestamp`
- `sender`
- `type`
- `content`

建议主键：

```text
chat + local_id
```

如果跨 DB shard 未来出现冲突，再补：

```text
chat + timestamp + local_id
```

### new-messages

`new-messages` 没有 `local_id`。

建议临时去重 key：

```text
username + sender + timestamp + type + hash(content)
```

风险：

- 同一个人在同一秒发送相同内容，可能发生碰撞。
- 如果需要强一致，增量消息入库后可再用 `history` 回查补 `local_id`。

## 8. 实时性验证当前状态

初始多次调用 `new-messages` 时，发现它不是直接从当前时刻开始，而是在追历史 backlog。

执行追赶：

```powershell
for ($i=1; $i -le 20; $i++) {
  $obj = wx new-messages -n 500 --json | ConvertFrom-Json
  ...
}
```

结果：

```text
batch=1 count=199 first=2026-05-18 22:32 last=2026-05-19 19:54 status=windowed
batch=2 count=0 status=windowed
```

结论：

- 当前 backlog 已追到空。
- 下一步可以做真正的实时性验证。
- 需要用户主动发送一条新的微信测试消息，然后立刻运行 `wx new-messages` 看延迟。

## 9. 实时性实测结果

用户在微信传输助手发送了一条测试消息。

拉取命令：

```powershell
Get-Date -Format 'yyyy-MM-dd HH:mm:ss'
wx new-messages -n 10 --json
```

拉取时间：

```text
2026-05-19 19:56:32
```

返回结果：

```json
{
  "count": 1,
  "messages": [
    {
      "chat": "filehelper",
      "chat_type": "private",
      "content": "hi",
      "is_group": false,
      "sender": "liangz77",
      "time": "2026-05-19 19:55",
      "timestamp": 1779191735,
      "type": "文本",
      "username": "filehelper"
    }
  ]
}
```

同时 `wx sessions -n 20 --json` 中，`filehelper` 成为最新会话：

```json
{
  "chat": "filehelper",
  "chat_type": "private",
  "is_group": false,
  "last_msg_type": "文本",
  "summary": "hi",
  "time": "05-19 19:55",
  "timestamp": 1779191735,
  "unread": 0,
  "username": "filehelper"
}
```

结论：

- `new-messages` 能读取到新发给微信传输助手的消息。
- 私聊增量消息可读。
- `filehelper` 在 `sessions` 中也能正确更新为最近会话。
- 这次测试从消息时间到拉取时间约 1 分钟内可见。由于本次不是秒表同步测试，只能证明准实时可用，不能精确证明秒级延迟。
- 下一轮需要连续发 3 到 5 条消息，每发一条立刻轮询，才能判断是否漏读、重复或乱序。

### 第二次轮询实测

用户再次向微信传输助手发送测试消息后，立即进行 10 轮轮询，每 2 秒一次。

轮询命令：

```powershell
for ($i=1; $i -le 10; $i++) {
  $now = Get-Date -Format 'yyyy-MM-dd HH:mm:ss.fff'
  $obj = wx new-messages -n 10 --json | ConvertFrom-Json
  ...
  Start-Sleep -Seconds 2
}
```

结果：

```text
poll=1 local_time=2026-05-19 19:58:28.380 count=1 status=windowed
poll=2 local_time=2026-05-19 19:58:30.907 count=0 status=windowed
poll=3 local_time=2026-05-19 19:58:33.016 count=0 status=windowed
poll=4 local_time=2026-05-19 19:58:35.114 count=0 status=windowed
poll=5 local_time=2026-05-19 19:58:37.216 count=0 status=windowed
poll=6 local_time=2026-05-19 19:58:39.329 count=0 status=windowed
poll=7 local_time=2026-05-19 19:58:41.458 count=0 status=windowed
poll=8 local_time=2026-05-19 19:58:43.554 count=0 status=windowed
poll=9 local_time=2026-05-19 19:58:45.643 count=0 status=windowed
poll=10 local_time=2026-05-19 19:58:47.752 count=0 status=windowed
```

第一轮返回消息：

```json
{
  "chat": "filehelper",
  "chat_type": "private",
  "content": "你好",
  "is_group": false,
  "sender": "liangz77",
  "time": "2026-05-19 19:57",
  "timestamp": 1779191866,
  "type": "文本",
  "username": "filehelper"
}
```

结论：

- 新消息在第一轮轮询中被读取到。
- 后续 9 轮没有重复返回，说明 `new-messages` 会推进增量状态。
- 本次未观察到重复。
- 本次只测了单条消息，尚未验证连续多条消息是否漏读或乱序。

### 连续 5 条消息实测

用户向微信传输助手连续发送 5 条消息：

```text
测试1
测试2
测试3
测试4
测试5
```

轮询命令：

```powershell
for ($i=1; $i -le 10; $i++) {
  $now = Get-Date -Format 'yyyy-MM-dd HH:mm:ss.fff'
  $obj = wx new-messages -n 20 --json | ConvertFrom-Json
  ...
  Start-Sleep -Seconds 2
}
```

结果：

```text
poll=1 local_time=2026-05-19 20:01:50.622 count=6 status=windowed
poll=2 local_time=2026-05-19 20:01:53.141 count=0 status=windowed
poll=3 local_time=2026-05-19 20:01:55.545 count=0 status=windowed
poll=4 local_time=2026-05-19 20:01:57.652 count=0 status=windowed
poll=5 local_time=2026-05-19 20:01:59.761 count=0 status=windowed
poll=6 local_time=2026-05-19 20:02:01.859 count=0 status=windowed
poll=7 local_time=2026-05-19 20:02:03.957 count=0 status=windowed
poll=8 local_time=2026-05-19 20:02:06.054 count=0 status=windowed
poll=9 local_time=2026-05-19 20:02:08.152 count=0 status=windowed
poll=10 local_time=2026-05-19 20:02:10.257 count=0 status=windowed
```

第一轮返回 6 条消息，其中 5 条是微信传输助手测试消息，1 条是同时进入增量窗口的群聊消息。

微信传输助手消息：

```text
测试1 timestamp=1779192079
测试2 timestamp=1779192081
测试3 timestamp=1779192082
测试4 timestamp=1779192084
测试5 timestamp=1779192086
```

同时捕获到 1 条群聊消息：

```text
chat=56042420718@chatroom
chat_type=group
timestamp=1779192100
```

结论：

- 连续 5 条私聊消息全部读取成功。
- 顺序正确，按 timestamp 递增。
- 未观察到漏读。
- 未观察到重复。
- 同一轮询窗口可能混入其他会话的新消息，说明 `new-messages` 是全局增量流，不是单会话增量流。
- 第一版入库必须按 `username/chat` 分会话处理，不能假设一次返回只属于当前测试会话。

## 10. 当前结论

可行：

- 读取最近会话。
- 读取增量消息。
- 读取群聊历史记录。
- 读取群成员。
- 读取部分私聊历史记录。
- 区分消息类型。
- 对文本、链接、图片、语音、文件做基础分类。

主要问题：

- `new-messages` 不返回 `local_id`。
- 私聊 `history` 的匹配规则不稳定。
- 实时性还需要用户发送新消息后验证。
- 图片和语音只能拿到占位信息，不能直接拿到可分析正文。

## 11. 对第一版 MVP 的影响

第一版建议：

- 以 `new-messages` 做增量入口。
- 以 `history` 做补充回查，尽量补 `local_id`。
- 先重点支持群聊和能稳定读取的私聊。
- AI 分析优先处理文本和链接。
- 图片、语音、文件先作为“非文本事件”进入摘要。
- 自动发送微信继续后置，不进入第一阶段。

下一步：

1. 用户发送一条测试微信消息。
2. 立即运行 `wx new-messages -n 10 --json`。
3. 记录从发送到读取的延迟。
4. 连续发 3 到 5 条，验证是否漏读、重复、乱序。
