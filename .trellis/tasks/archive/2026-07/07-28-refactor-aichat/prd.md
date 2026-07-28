# 优化重构 AI chat 部分

## Goal

对项目中 AI chat 相关代码进行结构化重构，降低单函数复杂度，消除重复逻辑，使代码更易维护、测试和扩展，同时保持外部行为不变。

## Requirements

1. 将 plugin/aichat/main.go 中巨大的 init() 处理函数拆分为职责清晰的小函数：条件判断、Agent 模式处理、普通聊天处理、消息发送等。
2. 将 plugin/llm/main.go 与 plugin/aichat/main.go 中重复出现的文本分块逻辑、LLM API 调用构造过程提取为可复用函数。
3. 规整 plugin/aichatcfg/main.go 中大量扁平的命令注册代码，使用统一注册辅助函数减少样板代码。
4. 保持所有现有命令、配置项、API 调用顺序与消息回复行为不变（外部行为兼容）。
5. 保留原有日志风格，避免引入新的依赖。
6. 不修改 github.com/FloatTech/zbputils/chat 等外部依赖包，只改动项目内代码。

## Acceptance Criteria

- [ ] go build ./... 通过（无新编译错误）。
- [ ] gofmt -w 格式化后无差异（或格式化后的代码）。
- [ ] 所有 AI chat 相关命令（随意聊天、@回复、Agent 模式、群总结、/gpt、配置命令）逻辑与重构前一致。
- [ ] plugin/aichat/main.go 的 init() 函数体显著缩小，复杂逻辑被抽到独立函数中。
- [ ] plugin/llm/main.go 中不再出现重复的文本分块逻辑。
- [ ] plugin/aichatcfg/main.go 的命令注册代码行数或重复样板明显减少。
- [ ] 代码审查未发现明显逻辑错误或边界回归。

## Notes

这是一个内部代码整理任务，不新增用户可见功能，也不改变配置格式。重点关注可读性、可维护性和重复代码消除。

