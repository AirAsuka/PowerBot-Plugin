package dict

// CampType 阵营类型，与 TOUE-Web toue_web/src/roles/rolesConfig.js 的 camp 字段保持一致
type CampType string

const (
	CampUnknown  CampType = "Unknown"
	CampCrewmate CampType = "Crewmate"
	CampImpostor CampType = "Impostor"
	CampNeutral  CampType = "Neutral"
)

// CampText 阵营中文名
var CampText = map[CampType]string{
	CampCrewmate: "船员阵营",
	CampImpostor: "伪装者阵营",
	CampNeutral:  "中立阵营",
}

// CampShortText 阵营短中文名，与 campTranslations.js 一致（用于职业行内标注）
var CampShortText = map[CampType]string{
	CampCrewmate: "船员",
	CampImpostor: "伪装者",
	CampNeutral:  "中立",
}

// RoleConfig 职业配置，移植自 TOUE-Web toue_web/src/roles/rolesConfig.js
// 未收录的职业使用默认值：Camp=CampUnknown, HasKill=false, HasTask=false
type RoleConfig struct {
	Camp    CampType
	HasKill bool
	HasTask bool
}

// roleConfigs 与 rolesConfig.js 一致，仅收录非默认值
var roleConfigs = map[string]RoleConfig{
	"Impostor": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"WolfLord": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Morphling": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Bomber": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Mimic": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Camouflager": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Poucher": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Butcher": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Miner": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Eraser": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Vampire": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Cleaner": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Undertaker": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Escapist": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Warlock": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Trickster": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"BountyHunter": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Terrorist": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Blackmailer": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Witch": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Ninja": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Yoyo": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"EvilTrapper": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Gambler": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Grenadier": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Gunsmith": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Berserker": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Marionette": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Gaoler": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Glitch": {Camp: CampImpostor, HasKill: true, HasTask: false},
	"Survivor": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"Amnisiac": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"Jester": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"Vulture": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"Lawyer": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"Executioner": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"Pursuer": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"Witness": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"BandLeader": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"PartTimer": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"Sidekick": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Pavlovsowner": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"Doomsayer": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Akujo": {Camp: CampNeutral, HasKill: false, HasTask: false},
	"Infected": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Jackal": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Pavlovsdogs": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Swooper": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Arsonist": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Werewolf": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"SchrodingersCat": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Thief": {Camp: CampNeutral, HasKill: false, HasTask: true},
	"Juggernaut": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Pelican": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Avenger": {Camp: CampNeutral, HasKill: true, HasTask: false},
	"Crewmate": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Vigilante": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Mayor": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Prosecutor": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Portalmaker": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Engineer": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Sheriff": {Camp: CampCrewmate, HasKill: true, HasTask: true},
	"Deputy": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"BodyGuard": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Jumper": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Detective": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Veteran": {Camp: CampCrewmate, HasKill: true, HasTask: true},
	"Medic": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Swapper": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Seer": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Hacker": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Tracker": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Snitch": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Spy": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"SecurityGuard": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Medium": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Trapper": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Prophet": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"InfoSleuth": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Balancer": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Redemptor": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Jailor": {Camp: CampCrewmate, HasKill: true, HasTask: true},
	"Oracle": {Camp: CampCrewmate, HasKill: false, HasTask: true},
	"Alchemyst": {Camp: CampCrewmate, HasKill: true, HasTask: true},
	"Dreamcatcher": {Camp: CampCrewmate, HasKill: true, HasTask: true},
}

// GetRoleConfig 获取职业配置，未收录时返回默认值（与 getRoleConfig 行为一致）
func GetRoleConfig(roleName string) RoleConfig {
	if cfg, ok := roleConfigs[roleName]; ok {
		return cfg
	}
	return RoleConfig{Camp: CampUnknown}
}

// GetCampText 获取阵营中文名，未知阵营返回原值
func GetCampText(camp CampType) string {
	if text, ok := CampText[camp]; ok {
		return text
	}
	return string(camp)
}

// GetCampShortText 获取阵营短中文名，未知阵营返回原值
func GetCampShortText(camp CampType) string {
	if text, ok := CampShortText[camp]; ok {
		return text
	}
	return string(camp)
}
