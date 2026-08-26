// Package aichat 大模型聊天和Agent
package aichat

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/RomiChan/syncx"
	goba "github.com/fumiama/go-onebot-agent"

	"github.com/FloatTech/zbputils/chat"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var (
	userAgents = syncx.Map[int64, *goba.Agent]{}
	// agentMemoryUsers 记录某个 Agent 会话当前对应的 QQ 号。
	agentMemoryUsers sync.Map
)

// aichatAgentOf 返回按 Bot 实例缓存的 Agent，其长期记忆使用按 QQ 号存储的 memberMemory。
func aichatAgentOf(id int64) *goba.Agent {
	if ag, ok := userAgents.Load(id); ok {
		return ag
	}
	ag := goba.NewAgent(chat.AgentCharConfig, 16, 8, time.Hour*24, "", getMemberMemory(), true, false)
	userAgents.Store(id, &ag)
	return &ag
}

// ResetAichatAgents 清空按 QQ 号长期记忆对应的 Agent 实例缓存。
func ResetAichatAgents() {
	userAgents.Range(func(id int64, _ *goba.Agent) bool {
		userAgents.Delete(id)
		return true
	})
}

func setAgentMemoryUser(grp, userID int64) {
	agentMemoryUsers.Store(grp, userID)
}

func unsetAgentMemoryUser(grp int64) {
	agentMemoryUsers.Delete(grp)
}

func agentMemoryUser(grp int64) int64 {
	if userID, ok := agentMemoryUsers.Load(grp); ok {
		return userID.(int64)
	}
	return grp
}

// aichatEvent 将 ZeroBot 事件转换成 Agent 可读的事件。
func aichatEvent(ctx *zero.Ctx) *goba.Event {
	msgid := int64(0)
	if id, ok := ctx.Event.MessageID.(int64); ok {
		msgid = id
	} else {
		return nil
	}
	msgd := ctx.Event.NativeMessage
	if len(msgd) > 1024 {
		msgs := message.ParseMessage(msgd)
		for _, m := range msgs {
			for k, v := range m.Data {
				if len(v) > 512 {
					m.Data[k] = v[:200] + " ... " + v[len(v)-200:]
				}
			}
		}
		raw, err := json.Marshal(&msgs)
		if err != nil {
			msgd = json.RawMessage(`[]`)
		} else {
			msgd = raw
		}
	}
	return &goba.Event{
		Time:        ctx.Event.Time,
		PostType:    ctx.Event.PostType,
		MessageType: ctx.Event.MessageType,
		SubType:     ctx.Event.SubType,
		MessageID:   msgid,
		GroupID:     ctx.Event.GroupID,
		UserID:      ctx.Event.UserID,
		TargetID:    ctx.Event.TargetID,
		SelfID:      ctx.Event.SelfID,
		NoticeType:  ctx.Event.NoticeType,
		OperatorID:  ctx.Event.OperatorID,
		File:        ctx.Event.File,
		RequestType: ctx.Event.RequestType,
		Flag:        ctx.Event.Flag,
		Comment:     ctx.Event.Comment,
		Sender:      ctx.Event.Sender,
		Message:     msgd,
	}
}

func init() {
	zero.OnMessage().FirstPriority().SetBlock(false).Handle(func(ctx *zero.Ctx) {
		gid := ctx.Event.GroupID
		if gid == 0 {
			gid = -ctx.Event.UserID
		}
		if !en.IsEnabledIn(gid) {
			return
		}
		ev := aichatEvent(ctx)
		if ev == nil {
			return
		}
		aichatAgentOf(ctx.Event.SelfID).AddEvent(gid, ev)
	})
}
