// Package undercover 提供群聊“谁是卧底”游戏。
package undercover

import (
	"fmt"
	"strconv"
	"strings"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const helpText = `谁是卧底（3—12人）
1. 创建卧底（创建者自动加入）
2. 其他玩家发送“加入卧底”
3. 房主发送“开始卧底”，机器人会私聊每个人的词
4. 按提示发送“卧底描述 你的描述”
5. 描述结束后发送“卧底投票 @玩家”

其他指令：卧底玩家、卧底状态、退出卧底、结束卧底
身份配置：5人起加入白板；8人起配置2狼并加入天使。
夜晚规则：投票后，所有普通拿词玩家私聊选择刀或不刀；狼刀人成功，平民开刀会自杀；天使和白板没有夜间行动。
胜负规则：所有狼出局则平民阵营胜；存活狼数达到其他存活人数时狼人阵营胜。
提示：开局前请先私聊机器人任意消息，确保机器人能发词。

管理员词库指令：
添加卧底词 词A|词B|分类|难度（难度1—3）
启用卧底词 ID / 禁用卧底词 ID
卧底词库 [页码] / 卧底词库统计`

var (
	engine = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault:  false,
		Brief:             "谁是卧底",
		Help:              helpText,
		PrivateDataFolder: "undercover",
	}).ApplySingle(ctxext.NewGroupSingle("本群上一条卧底指令还在处理中，请稍后再试"))
	rooms  = newRoomStore()
	wordDB = newWordLibrary(engine.DataFolder() + "words.db")
)

func init() {
	engine.OnFullMatchGroup([]string{"谁是卧底", "卧底帮助"}, zero.OnlyGroup).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text(helpText))
		})

	engine.OnFullMatch("创建卧底", zero.OnlyGroup).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			name := displayName(ctx, ctx.Event.UserID)
			_, err := rooms.create(ctx.Event.GroupID, ctx.Event.UserID, name)
			if err != nil {
				sendError(ctx, err)
				return
			}
			ctx.SendChain(
				message.At(ctx.Event.UserID),
				message.Text(" 已创建谁是卧底房间并自动加入！\n其他玩家请发送“加入卧底”，凑齐至少3人后由房主发送“开始卧底”。"),
			)
		})

	engine.OnFullMatch("加入卧底", zero.OnlyGroup).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			name := displayName(ctx, ctx.Event.UserID)
			var count int
			err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
				if err := g.join(ctx.Event.UserID, name); err != nil {
					return err
				}
				count = len(g.Players)
				return nil
			})
			if err != nil {
				sendError(ctx, err)
				return
			}
			ctx.SendChain(message.At(ctx.Event.UserID), message.Text(" 加入成功，当前玩家：", count, "/", maxPlayers))
		})

	engine.OnFullMatch("退出卧底", zero.OnlyGroup).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			var (
				room        *game
				remaining   int
				newHostID   int64
				newHostName string
			)
			err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
				room = g
				var err error
				newHostID, err = g.leave(ctx.Event.UserID)
				if err != nil {
					return err
				}
				remaining = len(g.Players)
				if newHostID != 0 {
					newHostName = g.Players[newHostID].Name
				}
				return nil
			})
			if err != nil {
				sendError(ctx, err)
				return
			}
			if remaining == 0 {
				rooms.removeIfSame(ctx.Event.GroupID, room)
				ctx.SendChain(message.Text("最后一名玩家已退出，房间已解散。"))
				return
			}
			text := fmt.Sprintf("退出成功，当前还剩%d人。", remaining)
			if newHostID != 0 {
				text += " 新房主是 " + newHostName + "。"
			}
			ctx.SendChain(message.Text(text))
		})

	engine.OnFullMatch("卧底玩家", zero.OnlyGroup).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			var text string
			err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
				var b strings.Builder
				fmt.Fprintf(&b, "谁是卧底玩家（%d/%d）：\n", len(g.Players), maxPlayers)
				for i, id := range g.JoinOrder {
					p := g.Players[id]
					mark := ""
					if id == g.HostID {
						mark = "（房主）"
					}
					if !p.Alive {
						mark += "（已出局）"
					}
					fmt.Fprintf(&b, "%d. %s%s\n", i+1, p.Name, mark)
				}
				text = strings.TrimSuffix(b.String(), "\n")
				return nil
			})
			if err != nil {
				sendError(ctx, err)
				return
			}
			ctx.SendChain(message.Text(text))
		})

	engine.OnFullMatch("开始卧底", zero.OnlyGroup).SetBlock(true).
		Handle(startGame)

	engine.OnRegex(`^卧底描述\s+([\s\S]+)$`, zero.OnlyGroup).SetBlock(true).
		Handle(handleClue)

	engine.OnRegex(votePattern, zero.OnlyGroup).SetBlock(true).
		Handle(handleVote)

	engine.OnFullMatch("卧底状态", zero.OnlyGroup).SetBlock(true).
		Handle(handleStatus)

	engine.OnFullMatch("结束卧底", zero.OnlyGroup).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			g, err := rooms.end(ctx.Event.GroupID, ctx.Event.UserID, zero.AdminPermission(ctx))
			if err != nil {
				sendError(ctx, err)
				return
			}
			text := "谁是卧底房间已结束。"
			if g.Phase != phaseLobby && len(g.WolfIDs) > 0 {
				text += "\n" + revealSummary(g, g.reveal())
			}
			ctx.SendChain(message.Text(text))
		})

	registerWordCommands()
	registerNightCommands()
}

func startGame(ctx *zero.Ctx) {
	if err := rooms.canBegin(ctx.Event.GroupID, ctx.Event.UserID); err != nil {
		sendError(ctx, err)
		return
	}
	pair, err := wordDB.randomPair()
	if err != nil {
		sendError(ctx, err)
		return
	}
	g, secrets, err := rooms.begin(ctx.Event.GroupID, ctx.Event.UserID, pair)
	if err != nil {
		sendError(ctx, err)
		return
	}

	sent := make([]int64, 0, len(secrets))
	var failed int64
	for _, item := range secrets {
		id := ctx.SendPrivateMessage(item.UserID, message.Text(secretText(item)))
		if id == 0 {
			failed = item.UserID
			break
		}
		sent = append(sent, item.UserID)
	}

	if failed != 0 {
		_ = rooms.finishDeal(ctx.Event.GroupID, g, false)
		for _, id := range sent {
			ctx.SendPrivateMessage(id, message.Text("【谁是卧底】因有玩家无法接收私聊，本次开局已取消，刚才的词语作废。"))
		}
		ctx.SendChain(
			message.Text("开局失败：无法私聊 "), message.At(failed),
			message.Text("。房间已取消，请该玩家先私聊机器人任意消息后重新创建。"),
		)
		return
	}
	if err := rooms.finishDeal(ctx.Event.GroupID, g, true); err != nil {
		for _, id := range sent {
			ctx.SendPrivateMessage(id, message.Text("【谁是卧底】房间在发词期间被结束，本次开局已取消，刚才的词语作废。"))
		}
		sendError(ctx, err)
		return
	}

	var (
		orderNames []string
		firstID    int64
	)
	_ = rooms.withRoom(ctx.Event.GroupID, func(current *game) error {
		for _, id := range current.Order {
			orderNames = append(orderNames, current.Players[id].Name)
		}
		firstID = current.currentDescriber()
		return nil
	})
	ctx.SendChain(
		message.Text("发词完成！", roleSetupText(len(secrets)), "\n第1轮描述顺序：\n", numberedNames(orderNames), "\n请 "),
		message.At(firstID), message.Text(" 先发送“卧底描述 你的描述”。"),
	)
}

func handleClue(ctx *zero.Ctx) {
	clue := ctx.State["regex_matched"].([]string)[1]
	var (
		nextID   int64
		nextName string
		voting   bool
	)
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		var err error
		nextID, voting, err = g.describe(ctx.Event.UserID, clue)
		if nextID != 0 && g.Players[nextID] != nil {
			nextName = g.Players[nextID].Name
		}
		return err
	})
	if err != nil {
		if err == errNotYourTurn && nextID != 0 {
			ctx.SendChain(message.Text("还没轮到你，请等待 "), message.At(nextID), message.Text("（", nextName, "）描述。"))
			return
		}
		sendError(ctx, err)
		return
	}
	if voting {
		ctx.SendChain(message.Text("本轮描述完毕，进入投票阶段。所有存活玩家请发送“卧底投票 @玩家”；可以改票，以最后一票为准。"))
		return
	}
	ctx.SendChain(message.Text("描述已记录，下一位请 "), message.At(nextID), message.Text("（", nextName, "）描述。"))
}

func handleVote(ctx *zero.Ctx) {
	matches := ctx.State["regex_matched"].([]string)
	target, err := voteTarget(matches)
	if err != nil {
		sendError(ctx, err)
		return
	}

	var (
		result         voteResult
		eliminatedName string
		tieNames       []string
		finalSummary   string
		room           *game
	)
	err = rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		room = g
		var voteErr error
		result, voteErr = g.vote(ctx.Event.UserID, target)
		if voteErr != nil {
			return voteErr
		}
		if result.Eliminated.ID != 0 {
			eliminatedName = g.Players[result.Eliminated.ID].Name
		}
		for _, id := range result.Tie {
			tieNames = append(tieNames, g.Players[id].Name)
		}
		if result.Winner != "" {
			finalSummary = revealSummary(g, result.Reveal)
		}
		return nil
	})
	if err != nil {
		sendError(ctx, err)
		return
	}

	if !result.Complete {
		action := "投票已记录"
		if result.Changed {
			action = "改票成功"
		}
		ctx.SendChain(message.Text(action, "（", result.VotesCast, "/", result.VotesNeeded, "）"))
		return
	}
	if len(result.Tie) > 0 {
		ctx.SendChain(message.Text("本轮平票：", strings.Join(tieNames, "、"), "。请所有存活玩家重新投票，本轮只能投给以上候选人。"))
		return
	}
	if result.Winner != "" {
		rooms.removeIfSame(ctx.Event.GroupID, room)
		ctx.SendChain(message.Text(
			eliminatedName, " 被投出，身份是", result.Eliminated.Role, "。\n",
			result.Winner, "阵营获胜！\n", finalSummary,
		))
		return
	}
	ctx.SendChain(message.Text(eliminatedName, " 被投出，身份是", result.Eliminated.Role, "。\n天黑请闭眼，机器人正在私聊本夜可行动的玩家。"))
	startNight(ctx, ctx.Event.GroupID, room, result.NightActors)
}

func handleStatus(ctx *zero.Ctx) {
	var text string
	err := rooms.withRoom(ctx.Event.GroupID, func(g *game) error {
		var b strings.Builder
		fmt.Fprintf(&b, "谁是卧底状态：%s\n", g.Phase)
		fmt.Fprintf(&b, "房主：%s\n", g.Players[g.HostID].Name)
		if g.Phase == phaseLobby {
			fmt.Fprintf(&b, "玩家：%d/%d（至少%d人开局）", len(g.Players), maxPlayers, minPlayers)
			text = b.String()
			return nil
		}
		fmt.Fprintf(&b, "轮次：第%d轮\n", g.Round)
		alive := g.alivePlayers()
		names := make([]string, 0, len(alive))
		for _, p := range alive {
			names = append(names, p.Name)
		}
		fmt.Fprintf(&b, "存活：%s", strings.Join(names, "、"))
		if g.Phase == phaseDescribing {
			fmt.Fprintf(&b, "\n当前描述：%s", g.Players[g.currentDescriber()].Name)
		}
		if g.Phase == phaseVoting {
			fmt.Fprintf(&b, "\n投票进度：%d/%d", len(g.Votes), len(g.Order))
			if len(g.VoteTargets) > 0 {
				candidates := make([]string, 0, len(g.VoteTargets))
				for _, id := range g.Order {
					if _, ok := g.VoteTargets[id]; ok {
						candidates = append(candidates, g.Players[id].Name)
					}
				}
				fmt.Fprintf(&b, "\n平票候选：%s", strings.Join(candidates, "、"))
			}
		}
		if g.Phase == phaseNight {
			fmt.Fprintf(&b, "\n夜间行动进度：%d/%d", len(g.NightActions), len(g.nightActors()))
		}
		text = b.String()
		return nil
	})
	if err != nil {
		sendError(ctx, err)
		return
	}
	ctx.SendChain(message.Text(text))
}

func displayName(ctx *zero.Ctx, id int64) string {
	name := strings.TrimSpace(ctx.CardOrNickName(id))
	if name == "" {
		return strconv.FormatInt(id, 10)
	}
	return name
}

func numberedNames(names []string) string {
	var b strings.Builder
	for i, name := range names {
		fmt.Fprintf(&b, "%d. %s", i+1, name)
		if i+1 < len(names) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func sendError(ctx *zero.Ctx, err error) {
	ctx.SendChain(message.Text("[谁是卧底] ", err.Error()))
}
