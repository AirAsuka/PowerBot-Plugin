// Package aichat 大模型聊天和Agent
package aichat

import (
	_ "embed"
	"strings"

	"github.com/FloatTech/floatbox/process"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

//go:embed commands.md
var commandKBFile string

const (
	// commandKBPrefix 知识库段落前缀，也用作“已注入”标记避免重复追加。
	commandKBPrefix = "\n\n【机器人指令知识库】\n"
	// commandKBDirective 告诉大模型如何使用指令知识库。
	commandKBDirective = "当用户询问“如何做某事”、“某个功能怎么用”、“有没有xx功能”，或表达想使用某个指令却记不清名字时，" +
		"请从下方【已启用】指令中挑选语义最匹配的一条，回复“可以试试发送「指令名」消息”，必要时补充参数示例（例如“可以试试发送「查看钱包余额」消息”）。" +
		"只推荐下方列出的已启用指令，不要编造不存在的指令。\n"
	// maxCommandKBRunes 知识库注入的最大长度保护。
	maxCommandKBRunes = 16000
)

// commandKBText 返回嵌入的指令知识库正文（带长度保护）。
func commandKBText() string {
	kb := strings.TrimSpace(commandKBFile)
	runes := []rune(kb)
	if len(runes) > maxCommandKBRunes {
		runes = runes[:maxCommandKBRunes]
	}
	return string(runes)
}

// commandKBSystemText 返回可拼接到系统提示词中的知识库段落。
func commandKBSystemText() string {
	return commandKBPrefix + commandKBDirective + commandKBText()
}

// splitRunes 按最大 rune 数切分文本，避免单条消息过长。
func splitRunes(s string, n int) []string {
	runes := []rune(s)
	var out []string
	for len(runes) > 0 {
		if len(runes) <= n {
			out = append(out, string(runes))
			break
		}
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}

func init() {
	// 指令大全: 直接查看本机器人全部指令知识库
	en.OnFullMatch("指令大全").SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			kb := commandKBText()
			if kb == "" {
				ctx.SendChain(message.Text("指令知识库为空"))
				return
			}
			for _, chunk := range splitRunes(kb, 1500) {
				ctx.SendChain(message.Text(chunk))
				process.SleepAbout1sTo2s()
			}
		})
}
