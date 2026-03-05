// Package chat 对话插件
package chat

import (
	"math/rand"
	"strconv"
	"time"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/extension/rate"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var (
	poke   = rate.NewManager[int64](time.Minute*5, 8) // 戳一戳
	engine = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "基础反应, 群空调",
		Help:             "chat\n- [BOT名字]\n- [戳一戳BOT]\n- 空调开\n- 空调关\n- 群温度\n- 设置温度[正整数]",
	})
)

func init() { // 插件主体
	// 被喊名字
	engine.OnFullMatch("", zero.OnlyToMe).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			var nickname = zero.BotConfig.NickName[0]
			time.Sleep(time.Second * 1)
			ctx.SendChain(message.Text(
				[]string{
					nickname + "在此，有何贵干~",
					"(っ●ω●)っ在~",
					"这里是" + nickname + "(っ●ω●)っ",
					nickname + "不在呢~",
				}[rand.Intn(4)],
			))
		})
	// 戳一戳
	engine.On("notice/notify/poke", zero.OnlyToMe).SetBlock(false).
		Handle(func(ctx *zero.Ctx) {
			pokerName := ctx.CardOrNickName(ctx.Event.UserID)
			switch {
			case poke.Load(ctx.Event.GroupID).AcquireN(3):
				// 5分钟共8块命令牌 一次消耗3块命令牌
				time.Sleep(time.Second * 1)
				ctx.SendChain(message.Text(randVoice(pokerName)))
			case poke.Load(ctx.Event.GroupID).Acquire():
				// 5分钟共8块命令牌 一次消耗1块命令牌
				time.Sleep(time.Second * 1)
				ctx.SendChain(message.Text(randVoice(pokerName)))
			default:
				// 频繁触发，不回复
			}
		})
	// 群空调
	var AirConditTemp = map[int64]int{}
	var AirConditSwitch = map[int64]bool{}
	engine.OnFullMatch("空调开").SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			AirConditSwitch[ctx.Event.GroupID] = true
			ctx.SendChain(message.Text("❄️哔~"))
		})
	engine.OnFullMatch("空调关").SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			AirConditSwitch[ctx.Event.GroupID] = false
			delete(AirConditTemp, ctx.Event.GroupID)
			ctx.SendChain(message.Text("💤哔~"))
		})
	engine.OnRegex(`设置温度(\d+)`).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			if _, exist := AirConditTemp[ctx.Event.GroupID]; !exist {
				AirConditTemp[ctx.Event.GroupID] = 26
			}
			if AirConditSwitch[ctx.Event.GroupID] {
				temp := ctx.State["regex_matched"].([]string)[1]
				AirConditTemp[ctx.Event.GroupID], _ = strconv.Atoi(temp)
				ctx.SendChain(message.Text(
					"❄️风速中", "\n",
					"群温度 ", AirConditTemp[ctx.Event.GroupID], "℃",
				))
			} else {
				ctx.SendChain(message.Text(
					"💤", "\n",
					"群温度 ", AirConditTemp[ctx.Event.GroupID], "℃",
				))
			}
		})
	engine.OnFullMatch(`群温度`).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			if _, exist := AirConditTemp[ctx.Event.GroupID]; !exist {
				AirConditTemp[ctx.Event.GroupID] = 26
			}
			if AirConditSwitch[ctx.Event.GroupID] {
				ctx.SendChain(message.Text(
					"❄️风速中", "\n",
					"群温度 ", AirConditTemp[ctx.Event.GroupID], "℃",
				))
			} else {
				ctx.SendChain(message.Text(
					"💤", "\n",
					"群温度 ", AirConditTemp[ctx.Event.GroupID], "℃",
				))
			}
		})
}
