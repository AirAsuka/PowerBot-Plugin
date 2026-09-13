package werewolf

import (
	"errors"
	"fmt"
	"math/rand"
	"slices"
)

type skillTurn struct {
	Actor int64
	Role  role
}
type skillResult struct {
	Text    string
	Outcome nightResult
}

func (g *game) resetAbilities() {
	g.Preparation = nil
	g.PreparationIndex = 0
	g.FearTarget, g.PreviousFear = 0, 0
	g.DreamTarget, g.PreviousDream = 0, 0
	g.CharmTarget, g.PreviousCharm = 0, 0
	g.GuardTargets, g.PreviousGuards = nil, nil
	g.ExtraDeaths, g.ExtraActed, g.GiftActed = nil, nil, nil
	g.GiftPoisons = nil
	g.LastExiled = 0
	g.ResumeDayOrder, g.ResumeSpeeches = nil, nil
	g.NoDayWords = false
}

func (g *game) prepareNight() {
	g.NightAnnounced, g.TeamNotified, g.GraveNotified = false, false, false
	g.Preparation = nil
	g.PreparationIndex = 0
	g.PreviousFear, g.FearTarget = g.FearTarget, 0
	g.PreviousDream, g.DreamTarget = g.DreamTarget, 0
	g.PreviousCharm, g.CharmTarget = g.CharmTarget, 0
	g.PreviousGuards, g.GuardTargets = g.GuardTargets, map[int64][]int64{}
	g.ExtraDeaths = map[int64]string{}
	g.ExtraActed, g.GiftActed = map[int64]bool{}, map[int64]bool{}
	g.GiftPoisons = nil
	g.SeerTarget = 0
	g.NoDayWords = false
	g.HunterDeaths = nil
	g.PendingHunter = 0
	// Information and protection are submitted before the collective wolf knife.
	// Deaths remain simultaneous at dawn, including pure seer / wolf wizard duels.
	for _, r := range []role{roleNightmare, rolePureSeer, roleMerchant, roleGargoyle, roleGuard, roleDreamer, roleWolfBeauty} {
		for _, id := range g.roleIDs(r, true) {
			if r == roleMerchant && g.Players[id].SkillUsed {
				continue
			}
			g.Preparation = append(g.Preparation, skillTurn{id, r})
		}
	}
	if len(g.Preparation) > 0 {
		g.Phase = phaseNightPrepare
	}
}

func (g *game) wolfTeam() []int64 {
	var ids []int64
	ordinary := 0
	for _, p := range g.Players {
		if p.Alive && p.Role.isWolf() && p.Role != roleGargoyle {
			ordinary++
		}
	}
	for _, id := range g.JoinOrder {
		p := g.Players[id]
		if p.Alive && p.Role.isWolf() && (p.Role != roleGargoyle || ordinary == 0) {
			ids = append(ids, id)
		}
	}
	return ids
}

func (g *game) canBroadcast(id int64) bool {
	if !g.Phase.isNight() || !slices.Contains(g.wolfTeam(), id) {
		return false
	}
	// Before the first fear action, do not reveal anyone to the nightmare.
	return !(g.Round == 1 && g.Phase == phaseNightPrepare && g.PreparationIndex == 0 && g.aliveRoleCount(roleNightmare) > 0)
}

func (g *game) currentPreparation() skillTurn {
	for g.PreparationIndex < len(g.Preparation) {
		turn := g.Preparation[g.PreparationIndex]
		if g.Players[turn.Actor].Alive && turn.Actor != g.FearTarget {
			return turn
		}
		g.PreparationIndex++
	}
	return skillTurn{}
}

func (g *game) advancePreparation() nightResult {
	if g.currentPreparation().Actor != 0 {
		return nightResult{}
	}
	g.Phase = phaseNightWolf
	g.WolfOrder = g.wolfTeam()
	g.WolfTurn = 0
	if len(g.WolfOrder) == 0 || g.Round == 1 && g.boardRules().FirstNightPeace || g.FearTarget != 0 && g.Players[g.FearTarget].Role.isWolf() {
		return g.completeWolfVotes(wolfVoteResult{Needed: len(g.WolfOrder)}).Outcome
	}
	return nightResult{}
}

func (g *game) frightenedRole(r role) bool {
	return g.FearTarget != 0 && g.Players[g.FearTarget].Role == r
}

func (g *game) specialComplete() bool {
	if !g.SeerActed || !g.WitchActed {
		return false
	}
	for _, id := range g.aliveIDs() {
		p := g.Players[id]
		if id == g.FearTarget {
			continue
		}
		if p.Role == roleWolfWizard && !g.ExtraActed[id] {
			return false
		}
		if p.HasGift && !p.GiftUsed && !g.GiftActed[id] {
			return false
		}
	}
	return true
}

// useSkill handles advanced roles and lucky gifts. All validation precedes mutation.
// Seer and witch keep their existing commands; all other private skills use this entry.
func (g *game) useSkill(actor int64, action string, target int64) (skillResult, error) {
	var out skillResult
	p := g.Players[actor]
	if p == nil {
		return out, errNotJoined
	}
	if !p.Alive {
		return out, errPlayerDead
	}
	if actor == g.FearTarget {
		return out, errors.New("你本夜被恐惧，不能行动")
	}
	gifted := action == "幸运" || action == "保留"
	r := p.Role
	if gifted {
		if g.Phase != phaseNightSpecial || !p.HasGift || p.GiftUsed || g.GiftActed[actor] {
			return out, errors.New("当前没有可使用的幸运儿技能")
		}
		r = p.Gift
	} else if g.Phase == phaseNightPrepare {
		turn := g.currentPreparation()
		if turn.Actor != actor {
			return out, errors.New("请等待私聊提示后行动")
		}
	} else if g.Phase == phaseNightSpecial && r == roleWolfWizard && !g.ExtraActed[actor] {
		// Wolf wizard acts after the knife.
	} else {
		return out, errors.New("现在没有你的特殊技能行动")
	}
	skip := action == "跳过" || action == "保留"
	if !skip {
		t := g.Players[target]
		if t == nil || !t.Alive {
			return out, errInvalidTarget
		}
		if target == actor && r != roleGuard {
			return out, errors.New("这个技能不能选择自己")
		}
		if r == roleGuard {
			if slices.Contains(g.PreviousGuards[actor], target) || slices.Contains(g.GuardTargets[actor], target) {
				return out, errors.New("不能守护上一夜或本夜已守护的目标")
			}
		}
		if r == roleNightmare && (target == g.PreviousFear || g.Round > 1 && t.Role.isWolf()) {
			return out, errors.New("不能连续恐惧同一人，也不能恐惧已知狼队友")
		}
		if r == roleWolfBeauty && target == g.PreviousCharm {
			return out, errors.New("不能连续两夜魅惑同一人")
		}
		if r == roleGargoyle && p.Inspected[target] {
			return out, errors.New("不能重复查验同一玩家")
		}
		if r == roleWolfWizard && t.Role.isWolf() {
			return out, errors.New("狼巫只能查验非狼人玩家")
		}
		if r == roleSeer && gifted && (p.Inspected[target] || g.SeerTarget == target && p.Role == roleSeer) {
			return out, errors.New("不能重复查验同一玩家")
		}
		if r == roleWitch && gifted && g.WitchPoison == target && p.Role == roleWitch {
			return out, errors.New("不能同夜重复毒同一目标")
		}
		if r == roleMerchant && action != "查验" && action != "毒药" && action != "守护" {
			return out, errors.New("商人格式：狼人杀技能 查验/毒药/守护 QQ号，或跳过")
		}
	} else if r == roleDreamer {
		return out, errors.New("摄梦人必须选择一名其他存活玩家")
	}
	out.Text = "技能行动已记录。"
	if !skip {
		switch r {
		case roleGuard:
			g.GuardTargets[actor] = append(g.GuardTargets[actor], target)
		case roleNightmare:
			g.FearTarget = target
		case roleDreamer:
			g.DreamTarget = target
		case roleWolfBeauty:
			g.CharmTarget = target
		case roleMerchant:
			gift := roleSeer
			if action == "毒药" {
				gift = roleWitch
			} else if action == "守护" {
				gift = roleGuard
			}
			p.SkillUsed = true
			if g.Players[target].Role.isWolf() {
				g.ExtraDeaths[actor] = "交易失败"
			} else {
				t := g.Players[target]
				t.HasGift, t.Gift, t.GiftUsed = true, gift, false
			}
		case rolePureSeer, roleWolfWizard, roleGargoyle, roleSeer:
			t := g.Players[target]
			if r == roleSeer {
				camp := "好人"
				if t.Role.isWolf() {
					camp = "狼人"
				}
				out.Text = fmt.Sprintf("查验结果：%s 是%s。", t.Name, camp)
			} else {
				out.Text = fmt.Sprintf("查验结果：%s 是%s。", t.Name, t.Role)
			}
			if r == roleGargoyle || r == roleSeer {
				if p.Inspected == nil {
					p.Inspected = map[int64]bool{}
				}
				p.Inspected[target] = true
			}
			if g.Round > 1 && (r == rolePureSeer && t.Role.isWolf() || r == roleWolfWizard && t.Role == rolePureSeer) {
				g.ExtraDeaths[target] = "查验出局"
			}
		case roleWitch:
			g.GiftPoisons = append(g.GiftPoisons, target)
		default:
			return skillResult{}, errors.New("该角色没有主动夜间技能")
		}
	}
	if gifted {
		g.GiftActed[actor] = true
		if !skip {
			p.GiftUsed = true
		}
	} else if g.Phase == phaseNightPrepare {
		g.PreparationIndex++
		out.Outcome = g.advancePreparation()
	} else {
		g.ExtraActed[actor] = true
	}
	if g.Phase == phaseNightSpecial && g.specialComplete() {
		out.Outcome = g.resolveNight()
	}
	g.touch()
	return out, nil
}

func (g *game) forcePreparation() skillResult {
	turn := g.currentPreparation()
	if turn.Actor == 0 {
		return skillResult{Outcome: g.advancePreparation()}
	}
	if turn.Role == roleDreamer {
		ids := slices.DeleteFunc(g.aliveIDs(), func(id int64) bool { return id == turn.Actor })
		if len(ids) > 0 {
			r, _ := g.useSkill(turn.Actor, "目标", ids[rand.Intn(len(ids))])
			return r
		}
	}
	r, _ := g.useSkill(turn.Actor, "跳过", 0)
	return r
}

func (g *game) protectedAtNight(id int64) bool {
	return id != 0 && (g.Players[id].Role == roleEvilKnight || g.DreamTarget == id)
}

func (g *game) nightDeaths() map[int64]string {
	deaths := map[int64]string{}
	add := func(id int64, cause string) {
		if id != 0 && !g.protectedAtNight(id) {
			deaths[id] = cause
		}
	}
	for id, cause := range g.ExtraDeaths {
		add(id, cause)
	}
	guarded := false
	for _, targets := range g.GuardTargets {
		if slices.Contains(targets, g.WolfVictim) {
			guarded = true
		}
	}
	healed := g.WolfVictim != 0 && g.WitchHeal == g.WolfVictim
	if guarded == healed {
		add(g.WolfVictim, "狼人袭击")
	}
	add(g.WitchPoison, "女巫毒杀")
	for _, id := range g.GiftPoisons {
		add(id, "女巫毒杀")
	}
	for _, id := range g.roleIDs(roleEvilKnight, true) {
		p := g.Players[id]
		if p.Reflected {
			continue
		}
		if g.SeerTarget == id {
			seers := g.roleIDs(roleSeer, true)
			if len(seers) > 0 {
				add(seers[0], "反伤")
				p.Reflected = true
			}
		} else if g.WitchPoison == id {
			witches := g.roleIDs(roleWitch, true)
			if len(witches) > 0 {
				add(witches[0], "反伤")
				p.Reflected = true
			}
		}
	}
	if g.DreamTarget != 0 {
		if g.DreamTarget == g.PreviousDream {
			deaths[g.DreamTarget] = "梦游"
		}
		for _, id := range g.roleIDs(roleDreamer, true) {
			if _, dead := deaths[id]; dead {
				deaths[g.DreamTarget] = "梦游"
			}
		}
	}
	// Charm can kill a dreamer, whose dream target must then also leave.
	for range len(g.Players) {
		before := len(deaths)
		if g.CharmTarget != 0 {
			for _, id := range g.roleIDs(roleWolfBeauty, true) {
				if _, dead := deaths[id]; dead {
					deaths[g.CharmTarget] = "殉情"
				}
			}
		}
		if g.DreamTarget != 0 {
			for _, id := range g.roleIDs(roleDreamer, true) {
				if _, dead := deaths[id]; dead {
					deaths[g.DreamTarget] = "梦游"
				}
			}
		}
		if len(deaths) == before {
			break
		}
	}
	return deaths
}

func (g *game) killDay(id int64, cause string) []death {
	p := g.Players[id]
	if p == nil || !p.Alive {
		return nil
	}
	p.Alive = false
	out := []death{{id, p.Role, cause}}
	if p.Role == roleWolfBeauty && cause != "决斗" && g.CharmTarget != 0 {
		out = append(out, g.killDay(g.CharmTarget, "殉情")...)
	}
	return out
}

func (g *game) canShoot(d death) bool {
	p := g.Players[d.ID]
	if p == nil || !p.Role.hasGun() || p.ShotUsed {
		return false
	}
	if d.ID == g.FearTarget && d.Cause == "狼人袭击" {
		return false
	}
	return d.Cause == "狼人袭击" || d.Cause == "放逐" || d.Cause == "开枪" || d.Cause == "猎人开枪" || d.Cause == "白狼王带走" || d.Cause == "决斗"
}

func (g *game) nextShooter() bool {
	for _, d := range g.HunterDeaths {
		if g.canShoot(d) {
			g.PendingHunter = d.ID
			g.Phase = phaseHunter
			return true
		}
	}
	return false
}

func (g *game) voterIDs() []int64 {
	return slices.DeleteFunc(g.aliveIDs(), func(id int64) bool { return g.Players[id].Revealed })
}

type daySkillResult struct {
	Deaths  []death
	Outcome hunterResult
}

func (g *game) daySkill(actor, target int64, want role) (daySkillResult, error) {
	var out daySkillResult
	if g.Phase != phaseDay {
		return out, errors.New("只能在白天发言阶段发动此技能")
	}
	p, t := g.Players[actor], g.Players[target]
	if p == nil {
		return out, errNotJoined
	}
	if !p.Alive {
		return out, errPlayerDead
	}
	if p.Role != want || p.SkillUsed {
		return out, errors.New("你没有可用的该技能")
	}
	if t == nil || !t.Alive || target == actor {
		return out, errInvalidTarget
	}
	if want != roleKnight && want != roleWhiteWolfKing {
		return out, errors.New("不支持的白天技能")
	}
	g.HunterFromNight, g.HunterContinueDay = false, false
	g.LastExiled = 0
	if want == roleKnight {
		p.SkillUsed = true
		if t.Role.isWolf() {
			out.Deaths = g.killDay(target, "决斗")
		} else {
			g.HunterContinueDay = true
			g.ResumeDayOrder = append([]int64(nil), g.DayOrder...)
			g.ResumeDayTurn = g.DayTurn
			g.ResumeSpeeches = append([]speech(nil), g.Speeches...)
			out.Deaths = g.killDay(actor, "决斗失败")
		}
	} else {
		p.SkillUsed = true
		out.Deaths = append(g.killDay(actor, "狼人自爆"), g.killDay(target, "白狼王带走")...)
		g.NoDayWords = true
	}
	if !g.HunterContinueDay {
		g.archiveCurrentSpeeches()
	}
	g.HunterDeaths = append([]death(nil), out.Deaths...)
	if g.nextShooter() {
		out.Outcome.NeedHunter = true
		return out, nil
	}
	g.beginDayLastWords(out.Deaths, nil, g.HunterContinueDay)
	if g.NoDayWords {
		result := g.finalizeDayLastWords()
		out.Outcome = hunterResult{Winner: result.Winner, Reveal: result.Reveal, StartNight: result.StartNight, FirstSpeaker: result.FirstSpeaker}
	} else {
		out.Outcome.AwaitingLastWords = true
	}
	return out, nil
}
