# 优化重构 AI chat 部分 — 技术设计

## 范围与边界

仅修改项目内以下文件：

- plugin/aichat/main.go（主重构对象）
- plugin/llm/main.go（提取公共辅助函数）
- plugin/aichatcfg/main.go（命令注册简化）

不修改任何外部依赖，包括 github.com/FloatTech/zbputils/chat、deepinfra、go-onebot-agent 等。

## 数据流与行为边界

- 输入：ZeroBot 消息事件 / 命令事件
- 中间处理：条件过滤 → 读取 chat.Storage / 全局 chat.AC 配置 → 构造 LLM 请求 → 发送回复
- 输出：调用 ctx.SendChain / ctx.Send / ctx.CallAction
- 关键边界：
  - 消息过滤条件（ctx.ExtractPlainText、概率、NoReplyAt、IsToMe 等）必须保持不变。
  - Agent 循环逻辑（最多 8 轮、请求/响应收集、goba.SVM 过滤）必须保持不变。
  - 普通聊天请求构造（chat.GetChatContext、系统提示开关）必须保持不变。
  - 回复发送流程（{name} / {me} 替换、{segment} 分割、语音记录尝试、@ 回复）必须保持不变。
  - 配置命令的权限与目标字段映射必须保持不变。

## 设计决策

### 1. 在 plugin/aichat/main.go 中拆分 init()

将当前 init() 内的处理函数拆分为：

- isAIChatEnabled(ctx, stor) bool：判断当前消息是否应触发 AI 聊天。
- esolveGID(ctx) int64：将私聊场景下的 GroupID 映射为负的 UserID。
- uildRole(ctx) goba.PermissionRole：根据管理员/超管身份计算 Agent 角色。
- unAgentChat(ctx, gid, temperature, topp, maxn) (handled bool)：执行 Agent 模式并返回是否已处理。
- unNormalChat(ctx, gid, temperature, topp, maxn)：执行普通大模型聊天。
- sendReply(ctx, gid, txt, replyToMe bool)：处理 {name} / {me} / {segment} 替换，尝试语音记录，并发送消息。

### 2. 在 plugin/llm/main.go 中复用辅助函数

新增 chunkText(text string, limit int) []string 函数，按字符上限优先在换行处切分文本。用于群总结和 /gpt 回复，替代两处重复的循环。

### 3. 在 plugin/aichatcfg/main.go 中统一前缀命令注册

新增 egisterExtraStr(prefix string, target *string, after ...func(*zero.Ctx)) 辅助函数，将 OnPrefix + NewExtraSetStr + 可选后置回调的重复模式压缩为一行。保留需要特殊处理（重置、查看）的单独注册。

### 4. 不引入新的包结构

为了保持改动最小、避免跨插件依赖，辅助函数全部放在各自的 main.go 同包内，以 internal 可见性（小写）提供。这样不会增加外部接口，也不会改变 go.mod。

## 风险与兼容性

- 风险：拆分过程中漏掉某个条件分支，导致概率触发或 @ 回复行为变化。
- 缓解：逐行对照原 init() 实现，确保每个条件与每个 ctx.Send 调用顺序一致。
- 风险：切分逻辑边界处理错误（如原本在 1000 字符处截断）。
- 缓解：用原逻辑等价实现，保留 strings.LastIndex(chunk, "\n") 的优先换行切分策略。
- 风险：配置命令注册辅助函数引入权限或匹配差异。
- 缓解：所有辅助函数仍使用 en.OnPrefix(...) 的原始参数签名。

## 验收时验证

1. go build ./... 通过。
2. gofmt -w plugin/aichat plugin/llm plugin/aichatcfg 后无变更。
3. 人工走查确认所有条件分支、字符串替换、日志字段与原代码一致。
