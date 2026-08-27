// Package aichat 大模型聊天和Agent
package aichat

import (
	"sync"

	"github.com/fumiama/deepinfra"
	dchat "github.com/fumiama/deepinfra/chat"
	"github.com/fumiama/deepinfra/model"

	"github.com/FloatTech/zbputils/chat"
	zero "github.com/wdvxdr1123/ZeroBot"
)

// directedChatKey 用于把群聊中不同用户的定向提问完全隔离。
type directedChatKey struct {
	groupID int64
	userID  int64
}

type directedChatItem string

func (i directedChatItem) String() string {
	return string(i)
}

// directedUserItem 用与全局聊天记录一致的格式标记一条定向提问。
func directedUserItem(name, text string) directedChatItem {
	return directedChatItem(chat.AtPrefix + chat.NameL + name + chat.NameR + text)
}

// groupUserItem 标记一条普通群聊消息，作为定向提问的背景上下文。
func groupUserItem(name, text string) directedChatItem {
	return directedChatItem(chat.NameL + name + chat.NameR + text)
}

var (
	directedChatLogs sync.Map
	groupChatLogs    sync.Map
)

// getDirectedChatLog 返回“群 + 用户”维度的定向提问上下文。
func getDirectedChatLog(groupID, userID int64) *dchat.Log[directedChatItem] {
	key := directedChatKey{groupID: groupID, userID: userID}
	if v, ok := directedChatLogs.Load(key); ok {
		return v.(*dchat.Log[directedChatItem])
	}
	log := dchat.NewLog[directedChatItem](16, 8, "\n\n", "")
	v, _ := directedChatLogs.LoadOrStore(key, &log)
	return v.(*dchat.Log[directedChatItem])
}

func getGroupChatLog(groupID int64) *dchat.Log[directedChatItem] {
	if v, ok := groupChatLogs.Load(groupID); ok {
		return v.(*dchat.Log[directedChatItem])
	}
	log := dchat.NewLog[directedChatItem](16, 8, "\n\n", "")
	v, _ := groupChatLogs.LoadOrStore(groupID, &log)
	return v.(*dchat.Log[directedChatItem])
}

// buildDirectedChatRequest 将用户自己的定向问答历史、近期普通群聊上下文和当前提问组合成一次请求。
// 普通群聊上下文放在当前提问之前，方便模型结合大家在讨论的话题。
func buildDirectedChatRequest(mod model.Protocol, groupID, userID int64, name, currentText, sysp string, noSystem bool) deepinfra.Model {
	combined := dchat.NewLog[directedChatItem](16, 8, "\n\n", "")

	history := dchat.Modelize(getDirectedChatLog(groupID, userID), 0, func(_ int, s string) directedChatItem {
		return directedChatItem(s)
	})
	if len(history) == 1 && history[0] == "" {
		history = nil
	}
	for i, item := range history {
		combined.Add(0, item, i%2 == 1)
	}

	groupHistory := dchat.Modelize(getGroupChatLog(groupID), 0, func(_ int, s string) directedChatItem {
		return directedChatItem(s)
	})
	if len(groupHistory) == 1 && groupHistory[0] == "" {
		groupHistory = nil
	}
	for _, item := range groupHistory {
		combined.Add(0, item, false)
	}

	combined.Add(0, directedUserItem(name, currentText), false)
	return combined.Modelize(mod, 0, sysp, noSystem)
}

// ResetDirectedChats 清空所有定向提问上下文和普通群聊上下文。
func ResetDirectedChats() {
	directedChatLogs.Range(func(key, _ any) bool {
		directedChatLogs.Delete(key)
		return true
	})
	groupChatLogs.Range(func(key, _ any) bool {
		groupChatLogs.Delete(key)
		return true
	})
}

func init() {
	zero.OnMessage(func(ctx *zero.Ctx) bool {
		if ctx.Event.GroupID == 0 || ctx.Event.UserID == ctx.Event.SelfID {
			return false
		}
		if ctx.ExtractPlainText() == "" || isAtSelf(ctx) {
			return false
		}
		return true
	}).FirstPriority().SetBlock(false).Handle(func(ctx *zero.Ctx) {
		name := ctx.Event.Sender.Name()
		if name == "" {
			name = ctx.CardOrNickName(ctx.Event.UserID)
		}
		getGroupChatLog(ctx.Event.GroupID).Add(0, groupUserItem(name, ctx.ExtractPlainText()), false)
	})
}
