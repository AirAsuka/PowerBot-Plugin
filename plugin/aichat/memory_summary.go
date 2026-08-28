// Package aichat 大模型聊天和Agent
package aichat

import (
	"strings"
	"sync"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	"github.com/sirupsen/logrus"

	"github.com/FloatTech/zbputils/chat"
)

const memorySummaryPrompt = "你是长期记忆整理器。请根据下面的新对话内容，更新该用户的长期记忆。\n要求：\n1. 只输出长期记忆内容本身，不要解释，不要加标题。\n2. 保留有长期价值的信息，例如用户身份、偏好、约定、重要事实；丢弃临时闲聊和寒暄。\n3. 总字数不要超过500个字。\n"

var memorySummaryLocks sync.Map

// maybeSummarizeMemory 在每次定向聊天结束后，让 AI 自动归纳该 QQ 号的长期记忆。
func maybeSummarizeMemory(userID int64, userText, replyText string) {
	lock, _ := memorySummaryLocks.LoadOrStore(userID, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	userText = strings.TrimSpace(userText)
	replyText = strings.TrimSpace(replyText)
	if userText == "" || replyText == "" {
		return
	}
	old := getMemberMemory().loadText(userID)
	prompt := memorySummaryPrompt
	if old != "" {
		prompt += "\n现有长期记忆：\n" + old
	}
	prompt += "\n用户说：\n" + userText
	prompt += "\nAI回复：\n" + replyText

	topp, maxn := chat.AC.MParams()
	mod, err := chat.AC.Type.Protocol(chat.AC.ModelName, 0.2, topp, maxn)
	if err != nil {
		logrus.Warnln("[aichat] summarize memory protocol err:", err)
		return
	}
	x := deepinfra.NewAPI(chat.AC.API, string(chat.AC.Key))
	data, err := x.Request(mod.User(model.NewContentText(prompt)))
	if err != nil {
		logrus.Warnln("[aichat] summarize memory err:", err)
		return
	}
	data = strings.TrimSpace(data)
	if data == "" {
		return
	}
	if err := getMemberMemory().set(userID, data); err != nil {
		logrus.Warnln("[aichat] save memory err:", err)
	}
}
