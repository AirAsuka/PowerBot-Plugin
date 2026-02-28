package dict

import (
	"strings"
)

// RoleCategories 职业阵营分类，key 为阵营中文名，value 为该阵营下的英文角色名列表
var RoleCategories = map[string][]string{
	"船员阵营": {
		"Crewmate", "Vigilante", "Mayor", "Prosecutor", "Portalmaker",
		"Engineer", "Sheriff", "Deputy", "Jailor", "BodyGuard",
		"Jumper", "Detective", "Veteran", "Medic", "Swapper",
		"Seer", "Hacker", "Tracker", "Snitch", "Prophet",
		"InfoSleuth", "Spy", "SecurityGuard", "Medium", "Trapper",
		"Balancer", "Redemptor", "Oracle",
	},
	"内鬼阵营": {
		"Impostor", "Morphling", "WolfLord", "Bomber", "Poucher",
		"Professional", "Butcher", "Mimic", "Camouflager", "Miner",
		"Eraser", "Vampire", "Cleaner", "Undertaker", "Marionette",
		"Warlock", "Trickster", "BountyHunter", "Terrorist", "Blackmailer",
		"Witch", "Ninja", "Yoyo", "EvilTrapper", "Gambler",
		"Grenadier", "Gunsmith", "Berserker",
	},
	"中立阵营": {
		"Survivor", "Amnisiac", "Jester", "Vulture", "Lawyer",
		"Executioner", "Pursuer", "PartTimer", "Jackal", "Sidekick",
		"Pavlovsowner", "Pavlovsdogs", "Infected", "Witness", "Swooper",
		"Arsonist", "Werewolf", "Thief", "Juggernaut", "Doomsayer",
		"Akujo", "Pelican", "BandLeader", "SchrodingersCat", "Avenger",
		"SoulSight",
	},
	"附加职业": {
		"Assassin", "Lover", "Disperser", "Vortox", "PoucherModifier",
		"ProfessionalModifier", "Specoality", "LastImpostor", "Bloody",
		"AntiTeleport", "Tiebreaker", "Aftermath", "Bait", "Sunglasses",
		"Torch", "Flash", "Multitasker", "Giant", "Mini",
		"Vip", "Indomitable", "Slueth", "Cursed", "Blind",
		"Watcher", "Radar", "Tunneler", "ButtonBarry", "Chameleon",
		"Shifter",
	},
	"幽灵类职业": {
		"Clog", "GhostEngineer", "Specter", "Poltergeist",
	},
}

// CategoryKeys 阵营名有序列表（用于展示）
var CategoryKeys = []string{"船员阵营", "内鬼阵营", "中立阵营", "附加职业", "幽灵类职业"}

// displayWidth 计算字符串的显示宽度（CJK 字符算 2，ASCII 算 1）
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r > 0x7F {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// padRight 用空格将字符串填充到指定显示宽度
func padRight(s string, width int) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-dw)
}

// GetCategoryRoles 获取指定阵营的格式化职业列表文本
func GetCategoryRoles(categoryName string) string {
	roles, ok := RoleCategories[categoryName]
	if !ok {
		return ""
	}

	// 翻译为中文名
	var names []string
	for _, en := range roles {
		if cn, ok := RoleText[en]; ok {
			names = append(names, cn)
		} else {
			names = append(names, en)
		}
	}

	// 计算最大显示宽度
	maxWidth := 0
	for _, name := range names {
		if w := displayWidth(name); w > maxWidth {
			maxWidth = w
		}
	}
	colWidth := maxWidth + 2 // 加 2 作为列间距

	// 每行 4 个
	const cols = 4
	var sb strings.Builder
	sb.WriteString("══ " + categoryName + " ══\n\n")
	for i, name := range names {
		if i > 0 && i%cols == 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(padRight(name, colWidth))
	}
	sb.WriteString("\n\n════════════════════════\n")
	sb.WriteString("💡 输入「小知识 职业名」查看详细描述")

	return sb.String()
}
