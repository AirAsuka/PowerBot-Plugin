# 优化重构 AI chat 部分 — 执行计划

## 任务清单

- [x] 1. 冻结当前代码：确认 git 工作区干净，必要时创建分支。
- [x] 2. 重构 `plugin/aichat/main.go`：拆分 `init()` 处理函数为多个小函数。
- [x] 3. 重构 `plugin/llm/main.go`：提取 `chunkText` 辅助函数，替换重复切分循环。
- [x] 4. 重构 `plugin/aichatcfg/main.go`：使用 `registerExtraStr` 简化字符串配置命令。
- [ ] 5. 格式化代码：`gofmt -w plugin/aichat plugin/llm plugin/aichatcfg`（当前环境无 Go，需用户执行）。
- [ ] 6. 编译验证：`go build ./...`（当前环境无 Go，需用户执行）。
- [x] 7. 走查检查：对照原代码确认逻辑等价性（已完成大括号/括号平衡检查）。
- [ ] 8. 提交并更新任务状态。

## 详细步骤

### 步骤 1：创建特性分支

```bash
git checkout -b codex/refactor-aichat
```

已完成，当前在 `codex/refactor-aichat` 分支。

### 步骤 2：重构 `plugin/aichat/main.go`

保持所有 import 不变。将当前 `init()` 内 `Handle` 的匿名函数替换为以下调用结构：

```go
en.OnMessage(chat.EnsureConfig, shouldHandle).SetBlock(false).Handle(func(ctx *zero.Ctx) {
    gid := resolveGID(ctx)
    stor := ctx.State[zero.StateKeyPrefixKeep+"aichatcfg_stor__"].(chat.Storage)
    temperature := stor.Temp()
    topp, maxn := chat.AC.MParams()
    mp := ctx.State[control.StateKeySyncxState].(*syncx.Map[string, any])

    if runAgentChat(ctx, gid, stor, temperature, topp, maxn, mp) {
        return
    }
    runNormalChat(ctx, gid, stor, temperature, topp, maxn)
})
```

新增函数（均不导出）：

- `shouldHandle(ctx *zero.Ctx) bool`
- `resolveGID(ctx *zero.Ctx) int64`
- `buildRole(ctx *zero.Ctx) goba.PermissionRole`
- `runAgentChat(ctx *zero.Ctx, gid int64, stor chat.Storage, temperature, topp, maxn float32, mp *syncx.Map[string, any]) bool`
- `runNormalChat(ctx *zero.Ctx, gid int64, stor chat.Storage, temperature, topp, maxn float32)`
- `sendReply(ctx *zero.Ctx, gid int64, stor chat.Storage, txt string, replyToMe bool)`

所有日志消息、字段名、替换规则、`process.SleepAbout1sTo2s()` 调用顺序与原代码一致。

### 步骤 3：重构 `plugin/llm/main.go`

新增：

```go
func chunkText(text string, limit int) []string {
    var chunks []string
    for len(text) > 0 {
        if len(text) <= limit {
            chunks = append(chunks, text)
            break
        }
        chunk := text[:limit]
        if last := strings.LastIndex(chunk, "\n"); last > 0 {
            chunk = text[:last+1]
        }
        chunks = append(chunks, chunk)
        text = text[len(chunk):]
    }
    return chunks
}
```

将群总结和 `/gpt` 中的重复循环替换为：

```go
for _, chunk := range chunkText(summaryText, 1000) {
    msg = append(msg, ctxext.FakeSenderForwardNode(ctx, message.Text(chunk)))
}
```

### 步骤 4：重构 `plugin/aichatcfg/main.go`

新增辅助函数：

```go
func registerExtraStr(prefix string, target *string, after ...zero.Handler) {
    handlers := append([]zero.Handler{chat.NewExtraSetStr(target)}, after...)
    en.OnPrefix(prefix, chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).Handle(handlers...)
}
```

将所有仅设置字符串的 `OnPrefix` 命令替换为 `registerExtraStr(...)` 调用。保留带复位逻辑和查看命令的单独处理。

### 步骤 5：格式化与编译

```bash
gofmt -w plugin/aichat/main.go plugin/llm/main.go plugin/aichatcfg/main.go
go build ./...
```

当前 WSL 环境未安装 Go，上述命令需由用户在具备 Go 的环境中执行。

### 步骤 6：检查与提交

- 检查 diff 是否仅包含结构变化，无逻辑变化。
- 提交信息：`refactor(aichat): split monolithic handlers and remove duplication`
- 推送分支：`git push origin codex/refactor-aichat`

## 回滚点

- 如编译失败，回退到上一个完整步骤的 git 状态，不要继续叠加修复。
- 如行为走查发现差异，优先恢复该函数到原实现，再重新拆分。

## 验证命令

```bash
gofmt -w plugin/aichat plugin/llm plugin/aichatcfg
go build ./...
```

## 代码审查重点

- `shouldHandle` 必须完整复现原条件顺序：storage 检查、Agent hooked 检查、文本非空检查、`NoReplyAt` 检查、概率检查、`IsToMe` 阻塞。
- `runAgentChat` 的 `for i := 0; i < 8; i++` 循环、`goba.SVM` 过滤、`CallAction` 响应封装必须完全一致。
- `sendReply` 的 `fastfailnorecord` 行为、语音记录尝试、`{segment}` 循环内的 `id` 复用必须完全一致。
- `chunkText` 的边界行为（恰好 limit 长度、无换行符）必须等价。
- `registerExtraStr` 不能改变命令注册的权限和匹配器。
