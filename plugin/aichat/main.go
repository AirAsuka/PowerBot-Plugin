// Package aichat 大模型聊天和Agent
package aichat

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	"github.com/RomiChan/syncx"
	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	goba "github.com/fumiama/go-onebot-agent"
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
	// en data [8 temp] [8 rate] LSB
	en = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Extra:            control.ExtraFromString("aichat"),
		Brief:            "大模型聊天和Agent",
		Help: "- (随意聊天, 概率匹配)\n" +
			"- 设置AI聊天长期记忆[QQ号 ]内容\n" +
			"- 查看AI聊天长期记忆[QQ号]\n" +
			"- 清除AI聊天长期记忆[QQ号]",

		PrivateDataFolder: "aichat",
	}).ApplySingle(single.New(
		single.WithKeyFn(func(ctx *zero.Ctx) int64 {
			if ctx.Event.GroupID == 0 {
				return -ctx.Event.UserID
			}
			return ctx.Event.GroupID
		}),
		// no post option, silently quit
	))
)

var (
	fastfailnorecord = false
)

const (
	privateChatDirective = "\n\n【回复要求】当前是私聊，请直接、自然地回复对方的问题。"
	atChatDirective      = "\n\n【回复要求】当前是群聊，最近一条以`>>`开头、直接@你的消息，是某位群友单独向你提问。请优先直接回答这条@你的问题，面向提问者回复；不要总结整个群聊，也不要逐一点名回复其他群友。只有当这个问题明显与上方群聊上下文相关时，才结合上下文并给出你的分析。"
	idleChatDirective    = "\n\n【回复要求】当前是群聊，没有用户直接@你，你是在群里主动插话。请结合最近的群聊上下文，说一句符合你身份、自然接得上话的内容；可以表达你对当前话题的观点、补充信息或适度提问。不要总结整个群聊，也不要逐一点名回复。如果长期记忆显示当前发言用户与你关系亲密、好感度较高，可以主动打招呼或关心对方。"
)

// isAtSelf 判断群聊消息是否真的直接@了机器人本人。
func isAtSelf(ctx *zero.Ctx) bool {
	if !ctx.Event.IsToMe {
		return false
	}
	return strings.Contains(ctx.Event.RawMessage, "[CQ:at,qq="+fmt.Sprint(ctx.Event.SelfID))
}

// aichatSystemDirective 根据当前消息类型返回本轮对话的行为要求。
func aichatSystemDirective(ctx *zero.Ctx, isDirected bool) string {
	if ctx.Event.GroupID == 0 {
		return privateChatDirective
	}
	if isDirected {
		return atChatDirective
	}
	return idleChatDirective
}

func init() {
	en.OnMessage(chat.EnsureConfig, func(ctx *zero.Ctx) bool {
		stor, ok := ctx.State[zero.StateKeyPrefixKeep+"aichatcfg_stor__"].(chat.Storage)
		if !ok {
			logrus.Warnln("ERROR: cannot get stor")
			return false
		}
		mp := ctx.State[control.StateKeySyncxState].(*syncx.Map[string, any])
		if _, ok := mp.Load(chat.StateKeyAgentHooked); !ok && !stor.NoAgent() {
			logrus.Infoln("[aichat] skip agent for ctx has not been hooked by agent")
			return false
		}
		plainText := ctx.ExtractPlainText()
		if plainText == "" {
			return false
		}
		if isMemoryCommand(plainText) {
			return false
		}
		gid := ctx.Event.GroupID
		isPrivate := gid == 0

		// 检查消息中是否真的@了机器人（通过原始消息判断）
		// ZeroBot 会将 [CQ:at,qq=xxx] 格式的@转换为 IsToMe=true，但需要防止误判
		isReallyToMe := isAtSelf(ctx)
		// logrus.Infoln("[aichat] @消息检测: isReallyToMe=", isReallyToMe, "NoReplyAt=", stor.NoReplyAt())

		if isPrivate {
			// 私聊：每条都响应
			ctx.Block()
			return true
		}

		// 群聊
		if isReallyToMe {
			// 真正@了机器人：检查 NoReplyAt 配置
			if stor.NoReplyAt() {
				return false
			}
			ctx.Block()
			return true
		}

		// 普通消息：检查概率
		rate := stor.Rate()
		if rate == 0 || rand.Intn(100) >= int(rate) {
			return false
		}
		return true
	}).SetBlock(false).Handle(func(ctx *zero.Ctx) {
		gid := ctx.Event.GroupID
		if gid == 0 {
			gid = -ctx.Event.UserID
		}
		stor := ctx.State[zero.StateKeyPrefixKeep+"aichatcfg_stor__"].(chat.Storage)
		temperature := stor.Temp()
		topp, maxn := chat.AC.MParams()
		mp := ctx.State[control.StateKeySyncxState].(*syncx.Map[string, any])
		isDirected := gid == 0 || isAtSelf(ctx)
		directive := aichatSystemDirective(ctx, isDirected)

		logrus.Debugln("[aichat] agent mode test: noagent", stor.NoAgent(), "hasapi", chat.AC.AgentAPI != "", "hasmodel", chat.AC.AgentModelName != "")
		if !stor.NoAgent() && chat.AC.AgentAPI != "" && chat.AC.AgentModelName != "" && chat.AC.Key != "" {
			logrus.Debugln("[aichat] enter agent mode")
			x := deepinfra.NewAPI(chat.AC.AgentAPI, string(chat.AC.AgentKey))
			mod, err := chat.AC.Type.Protocol(chat.AC.AgentModelName, temperature, topp, maxn)
			if err != nil {
				logrus.Warnln("ERROR: ", err)
				return
			}
			role := goba.PermRoleUser
			if zero.AdminPermission(ctx) {
				role = goba.PermRoleAdmin
				if zero.SuperUserPermission(ctx) {
					role = goba.PermRoleOwner
				}
			}
			ag := aichatAgentOf(ctx.Event.SelfID)
			logrus.Debugln("[aichat] got agent")
			setAgentMemoryUser(gid, ctx.Event.UserID)
			defer unsetAgentMemoryUser(gid)
			if chat.AC.ImageAPI != "" && !ag.CanViewImage() {
				mod, err := chat.AC.ImageType.Protocol(chat.AC.ImageModelName, temperature, topp, maxn)
				if err != nil {
					logrus.Warnln("ERROR: ", err)
					return
				}
				ag.SetViewImageAPI(deepinfra.NewAPI(chat.AC.ImageAPI, string(chat.AC.ImageKey)), mod)
				logrus.Debugln("[aichat] agent set img")
			}
			ctx.NoTimeout()
			logrus.Debugln("[aichat] agent set no timeout")
			hasresp := false
			for i := 0; i < 8; i++ { // 最大运行 8 轮因为问答上下文只有 16
				reqs := chat.CallAgent(ag, zero.SuperUserPermission(ctx), i+1, x, mod, gid, role)
				if len(reqs) == 0 {
					logrus.Debugln("[aichat] agent call got empty response")
					break
				}
				hasresp = true
				mp.Store(chat.StateKeyAgentTriggered, struct{}{})
				for _, req := range reqs {
					if req.Action == goba.SVM { // is a fake action
						continue
					}
					logrus.Debugln("[chat] agent triggered", gid, "add requ:", &req)
					ag.AddRequest(gid, &req)
					rsp := ctx.CallAction(req.Action, req.Params)
					logrus.Debugln("[chat] agent triggered", gid, "add resp:", &rsp)
					ag.AddResponse(gid, &goba.APIResponse{
						Status:  rsp.Status,
						Data:    json.RawMessage(rsp.Data.Raw),
						Message: rsp.Message,
						Wording: rsp.Wording,
						RetCode: rsp.RetCode,
					})
				}
			}
			if hasresp {
				return
			}
			// no response, fall back to normal chat
			logrus.Debugln("[aichat] agent fell back to normal chat")
		}

		x := deepinfra.NewAPI(chat.AC.API, string(chat.AC.Key))
		mod, err := chat.AC.Type.Protocol(chat.AC.ModelName, temperature, topp, maxn)
		if err != nil {
			logrus.Warnln("ERROR: ", err)
			return
		}

		// 提取消息中的图片URL
		var imageURLs []string
		for _, seg := range ctx.Event.Message {
			if seg.Type == "image" {
				if url := seg.Data["url"]; url != "" {
					imageURLs = append(imageURLs, url)
				}
			}
		}

		var data string
		if len(imageURLs) > 0 && chat.AC.ImageAPI != "" && chat.AC.ImageModelName != "" {
			// 识图模式：发送图片+文本
			logrus.Debugln("[aichat] 识图模式, 图片数量:", len(imageURLs))
			imgAPI := deepinfra.NewAPI(chat.AC.ImageAPI, string(chat.AC.ImageKey))
			imgMod, imgErr := chat.AC.ImageType.Protocol(chat.AC.ImageModelName, temperature, topp, maxn)
			if imgErr != nil {
				logrus.Warnln("ERROR: ", imgErr)
				return
			}
			contents := make([]model.Content, 0, len(imageURLs)+1)
			for _, url := range imageURLs {
				contents = append(contents, model.NewContentImageURL(url))
			}
			plainText := ctx.ExtractPlainText()
			userPrompt := strings.TrimSpace(plainText + memorySystemText(ctx.Event.UserID) + directive)
			if userPrompt != "" {
				contents = append(contents, model.NewContentText(userPrompt))
			}
			data, err = imgAPI.Request(imgMod.User(contents...))
		} else {
			// 普通文本聊天
			sysp := chat.AC.SystemP + memorySystemText(ctx.Event.UserID) + directive
			data, err = x.Request(chat.GetChatContext(mod, gid, sysp, bool(chat.AC.NoSystemP)))
		}
		if err != nil {
			logrus.Warnln("[aichat] post err:", err)
			return
		}

		txt := chat.Sanitize(strings.Trim(data, "\n 　"))
		if len(txt) > 0 {
			chat.AddChatReply(gid, txt)
			nick := zero.BotConfig.NickName[rand.Intn(len(zero.BotConfig.NickName))]
			txt = strings.ReplaceAll(txt, "{name}", ctx.CardOrNickName(ctx.Event.UserID))
			txt = strings.ReplaceAll(txt, "{me}", nick)
			id := any(nil)
			if isDirected {
				id = ctx.Event.MessageID
			}
			for _, t := range strings.Split(txt, "{segment}") {
				if t == "" {
					continue
				}
				logrus.Debugln("[aichat] 回复内容:", t)
				recCfg := airecord.GetConfig()
				record := ""
				if !fastfailnorecord && !stor.NoRecord() {
					record = ctx.GetAIRecord(recCfg.ModelID, recCfg.Customgid, t)
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
		}
	})
}
