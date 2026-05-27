// Package aichat 大模型聊天 - 对接 PowerBot AI Backend
package aichat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/extension/single"
	"github.com/wdvxdr1123/ZeroBot/message"

	"github.com/FloatTech/AnimeAPI/airecord"
	"github.com/FloatTech/floatbox/process"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/control"
)

var (
	en = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Extra:            control.ExtraFromString("aichat"),
		Brief:            "大模型聊天和Agent",
		Help:             "- (随意聊天, 概率匹配)",

		PrivateDataFolder: "aichat",
	}).ApplySingle(single.New(
		single.WithKeyFn(func(ctx *zero.Ctx) int64 {
			if ctx.Event.GroupID == 0 {
				return -ctx.Event.UserID
			}
			return ctx.Event.GroupID
		}),
	))
)

var (
	fastfailnorecord = false
	// BackendURL Python AI Backend 地址
	BackendURL = "http://127.0.0.1:8000"
)

// logClient 用于 fire-and-forget 消息日志的轻量 HTTP 客户端
var logClient = &http.Client{Timeout: 5 * time.Second}

// logMessageAsync 火后即忘：将消息记录到后端作为上下文
func logMessageAsync(sessionID, userID, userName, plainText string, images []string) {
	go func() {
		body, _ := json.Marshal(ChatRequest{
			SessionID: sessionID,
			UserID:    userID,
			UserName:  userName,
			Message:   plainText,
			Images:    images,
		})
		url := strings.TrimRight(BackendURL, "/") + "/api/v1/messages/log"
		resp, err := logClient.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			logrus.Debugln("[aichat] log message err:", err)
			return
		}
		resp.Body.Close()
	}()
}

func init() {
	aiClient := NewAIChatClient(BackendURL)

	en.OnMessage(chat.EnsureConfig, func(ctx *zero.Ctx) bool {
		stor, ok := ctx.State[zero.StateKeyPrefixKeep+"aichatcfg_stor__"].(chat.Storage)
		if !ok {
			logrus.Warnln("[aichat] ERROR: cannot get stor")
			return false
		}
		plainText := ctx.ExtractPlainText()
		if plainText == "" {
			return false
		}
		gid := ctx.Event.GroupID
		isPrivate := gid == 0

		sessionID := fmt.Sprintf("group_%d", gid)
		if gid == 0 {
			sessionID = fmt.Sprintf("group_%d", -ctx.Event.UserID)
		}

		// 提取图片URL（用于日志和请求）
		var imageURLs []string
		for _, seg := range ctx.Event.Message {
			if seg.Type == "image" {
				if url := seg.Data["url"]; url != "" {
					imageURLs = append(imageURLs, url)
				}
			}
		}

		// 检查是否真的 @ 了机器人
		isReallyToMe := false
		if ctx.Event.IsToMe {
			rawMsg := ctx.Event.RawMessage
			if strings.Contains(rawMsg, "[CQ:at,qq="+fmt.Sprint(ctx.Event.SelfID)) {
				isReallyToMe = true
			}
		}

		trigger := false
		if isPrivate {
			trigger = true
		} else if isReallyToMe {
			trigger = !stor.NoReplyAt()
		} else {
			rate := stor.Rate()
			trigger = rate > 0 && rand.Intn(100) < int(rate)
		}

		if !trigger {
			// 不触发回复，但记录此消息作为后续对话的上下文
			logMessageAsync(sessionID, fmt.Sprint(ctx.Event.UserID),
				ctx.CardOrNickName(ctx.Event.UserID), plainText, imageURLs)
			return false
		}

		ctx.Block()
		return true
	}).SetBlock(false).Handle(func(ctx *zero.Ctx) {
		gid := ctx.Event.GroupID
		if gid == 0 {
			gid = -ctx.Event.UserID
		}

		// 提取图片URL
		var imageURLs []string
		for _, seg := range ctx.Event.Message {
			if seg.Type == "image" {
				if url := seg.Data["url"]; url != "" {
					imageURLs = append(imageURLs, url)
				}
			}
		}

		plainText := ctx.ExtractPlainText()
		sessionID := fmt.Sprintf("group_%d", gid)

		req := &ChatRequest{
			SessionID: sessionID,
			UserID:    fmt.Sprint(ctx.Event.UserID),
			UserName:  ctx.CardOrNickName(ctx.Event.UserID),
			Message:   plainText,
			Images:    imageURLs,
			UseRAG:    false,
		}

		ctxNoTimeout, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		resp, err := aiClient.Chat(ctxNoTimeout, req)
		if err != nil {
			logrus.Warnln("[aichat] backend error:", err)
			return
		}

		txt := strings.Trim(resp.Reply, "\n 　")
		if len(txt) == 0 {
			return
		}

		nick := zero.BotConfig.NickName[rand.Intn(len(zero.BotConfig.NickName))]
		txt = strings.ReplaceAll(txt, "{name}", ctx.CardOrNickName(ctx.Event.UserID))
		txt = strings.ReplaceAll(txt, "{me}", nick)

		stor := ctx.State[zero.StateKeyPrefixKeep+"aichatcfg_stor__"].(chat.Storage)
		id := any(nil)
		if ctx.Event.IsToMe {
			id = ctx.Event.MessageID
		}
		for _, t := range strings.Split(txt, "{segment}") {
			if t == "" {
				continue
			}
			logrus.Debugln("[aichat] 回复内容:", t)
			recCfg := airecord.GetConfig()
			if !fastfailnorecord && !stor.NoRecord() {
				record := ctx.GetAIRecord(recCfg.ModelID, recCfg.Customgid, t)
				if record != "" {
					ctx.SendChain(message.Record(record))
					continue
				}
				fastfailnorecord = true
			}
			if id != nil {
				id = ctx.SendChain(message.Reply(id), message.Text(t))
			} else {
				id = ctx.SendChain(message.Text(t))
			}
			process.SleepAbout1sTo2s()
		}
	})
}
