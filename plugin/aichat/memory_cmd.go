// Package aichat 大模型聊天和Agent
package aichat

import (
	"errors"
	"strconv"
	"strings"
	"unicode"

	"github.com/FloatTech/zbputils/chat"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func isMemoryCommand(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "设置AI聊天长期记忆") ||
		strings.HasPrefix(text, "查看AI聊天长期记忆") ||
		strings.HasPrefix(text, "清除AI聊天长期记忆")
}

func parseMemoryArgs(ctx *zero.Ctx, args string, needText bool) (target int64, text string, err error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return ctx.Event.UserID, "", nil
	}
	i := strings.IndexFunc(args, unicode.IsSpace)
	if i > 0 {
		first := args[:i]
		if qq, e := strconv.ParseInt(first, 10, 64); e == nil && qq > 0 {
			rest := strings.TrimSpace(args[i:])
			if needText && rest == "" {
				return 0, "", errors.New("长期记忆内容不能为空")
			}
			return qq, rest, nil
		}
	}
	return ctx.Event.UserID, args, nil
}

func init() {
	en.OnPrefix("设置AI聊天长期记忆", chat.EnsureConfig).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		target, text, err := parseMemoryArgs(ctx, ctx.State["args"].(string), true)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		if target != ctx.Event.UserID && !zero.SuperUserPermission(ctx) {
			ctx.SendChain(message.Text("ERROR: 只有超级用户才能为其他成员设置长期记忆"))
			return
		}
		if text == "" {
			ctx.SendChain(message.Text("用法: 设置AI聊天长期记忆[QQ号 ]内容"))
			return
		}
		if err := getMemberMemory().append(target, text); err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Text("已保存长期记忆"))
	})

	en.OnPrefix("查看AI聊天长期记忆", chat.EnsureConfig).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		target, _, err := parseMemoryArgs(ctx, ctx.State["args"].(string), false)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		if target != ctx.Event.UserID && !zero.SuperUserPermission(ctx) {
			ctx.SendChain(message.Text("ERROR: 只有超级用户才能查看其他成员的长期记忆"))
			return
		}
		mem := getMemberMemory().loadText(target)
		if mem == "" {
			ctx.SendChain(message.Text("该成员暂无长期记忆"))
			return
		}
		ctx.SendChain(message.Text(mem))
	})

	en.OnPrefix("清除AI聊天长期记忆", chat.EnsureConfig).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		target, _, err := parseMemoryArgs(ctx, ctx.State["args"].(string), false)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		if target != ctx.Event.UserID && !zero.SuperUserPermission(ctx) {
			ctx.SendChain(message.Text("ERROR: 只有超级用户才能清除其他成员的长期记忆"))
			return
		}
		if err := getMemberMemory().clear(target); err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Text("已清除长期记忆"))
	})
}
