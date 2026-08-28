package greeter

import (
	"strconv"
	"strings"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var engine = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
	DisableOnDefault:  false,
	Brief:             "入群欢迎/退群欢送",
	Help:              "- 设置入群消息<内容>\n- 设置退群消息<内容>\n- 清除入群消息\n- 清除退群消息\n- 测试入群消息\n- 测试退群消息\n- 查看入退群消息\n可用变量: {at} {nickname} {uid} {gid} {groupname} {avatar}\n内容支持CQ码, 如 [CQ:image,file=图片url]",
	PrivateDataFolder: "greeter",
})

func init() {
	// 有人入群时发送欢迎消息
	engine.OnNotice(func(ctx *zero.Ctx) bool {
		return ctx.Event.NoticeType == "group_increase" && ctx.Event.SelfID != ctx.Event.UserID
	}).SetBlock(false).Handle(func(ctx *zero.Ctx) {
		c := load(ctx.Event.GroupID)
		if strings.TrimSpace(c.Welcome) != "" {
			send(ctx, c.Welcome)
		}
	})

	// 有人退群时发送欢送消息
	engine.OnNotice(func(ctx *zero.Ctx) bool {
		return ctx.Event.NoticeType == "group_decrease" && ctx.Event.SelfID != ctx.Event.UserID
	}).SetBlock(false).Handle(func(ctx *zero.Ctx) {
		c := load(ctx.Event.GroupID)
		if strings.TrimSpace(c.Farewell) != "" {
			send(ctx, c.Farewell)
		}
	})

	// 设置入群消息
	engine.OnRegex(`^设置入群消息([\s\S]*)$`, zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			text := ctx.State["regex_matched"].([]string)[1]
			if strings.TrimSpace(text) == "" {
				ctx.SendChain(message.Text("请输入消息内容, 例如: 设置入群消息 欢迎 {at} 加入本群~"))
				return
			}
			c := load(ctx.Event.GroupID)
			c.Welcome = text
			if err := save(ctx.Event.GroupID, c); err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("已设置本群入群消息:\n", render(ctx, text)))
		})

	// 设置退群消息
	engine.OnRegex(`^设置退群消息([\s\S]*)$`, zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			text := ctx.State["regex_matched"].([]string)[1]
			if strings.TrimSpace(text) == "" {
				ctx.SendChain(message.Text("请输入消息内容, 例如: 设置退群消息 {nickname} 离开了我们..."))
				return
			}
			c := load(ctx.Event.GroupID)
			c.Farewell = text
			if err := save(ctx.Event.GroupID, c); err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("已设置本群退群消息:\n", render(ctx, text)))
		})

	// 清除入群消息(恢复默认)
	engine.OnFullMatch("清除入群消息", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			c := load(ctx.Event.GroupID)
			c.Welcome = defaultWelcome
			if err := save(ctx.Event.GroupID, c); err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("已清除本群入群消息, 恢复默认: ", defaultWelcome))
		})

	// 清除退群消息(恢复默认)
	engine.OnFullMatch("清除退群消息", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			c := load(ctx.Event.GroupID)
			c.Farewell = defaultFarewell
			if err := save(ctx.Event.GroupID, c); err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("已清除本群退群消息, 恢复默认: ", defaultFarewell))
		})

	// 测试入群消息
	engine.OnFullMatch("测试入群消息", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			send(ctx, load(ctx.Event.GroupID).Welcome)
		})

	// 测试退群消息
	engine.OnFullMatch("测试退群消息", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			send(ctx, load(ctx.Event.GroupID).Farewell)
		})

	// 查看本群配置
	engine.OnFullMatch("查看入退群消息", zero.OnlyGroup).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			c := load(ctx.Event.GroupID)
			ctx.SendChain(message.Text(
				"本群入群消息: ", c.Welcome,
				"\n渲染示例: ", render(ctx, c.Welcome),
				"\n本群退群消息: ", c.Farewell,
				"\n渲染示例: ", render(ctx, c.Farewell),
			))
		})
}

// send 将配置文本渲染后发送到当前群
func send(ctx *zero.Ctx, text string) {
	msg := message.ParseMessageFromString(render(ctx, text))
	if len(msg) == 0 {
		msg = message.Message{message.Text(text)}
	}
	ctx.SendGroupMessage(ctx.Event.GroupID, msg)
}

// render 将占位符替换为实际内容, 并返回可解析的CQ码字符串
func render(ctx *zero.Ctx, text string) string {
	uid := strconv.FormatInt(ctx.Event.UserID, 10)
	nickname := ctx.CardOrNickName(ctx.Event.UserID)
	if nickname == "" {
		nickname = uid
	}
	gid := strconv.FormatInt(ctx.Event.GroupID, 10)
	groupname := ctx.GetThisGroupInfo(true).Name
	return strings.NewReplacer(
		"{at}", "[CQ:at,qq="+uid+"]",
		"{nickname}", nickname,
		"{uid}", uid,
		"{gid}", gid,
		"{groupname}", groupname,
		"{avatar}", "[CQ:image,file=https://q4.qlogo.cn/g?b=qq&nk="+uid+"&s=640]",
	).Replace(text)
}
