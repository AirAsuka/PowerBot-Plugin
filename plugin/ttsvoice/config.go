// Package ttsvoice MiniMax异步语音合成
package ttsvoice

import (
	"fmt"
	"strings"
	"sync"

	sql "github.com/FloatTech/sqlite"
	"github.com/FloatTech/zbputils/img/text"
)

// 默认配置
const (
	DefaultAPIURL  = "https://api.minimax.chat/v1"
	DefaultModel   = "speech-2.8-hd"
	DefaultVoiceID = "female-shaonv"
	DefaultSpeed   = 1.0
	DefaultVolume  = 10
	DefaultPitch   = 1.0
)

// VoiceList 音色列表
var VoiceList = []struct {
	ID   string
	Name string
}{
	{"male-qn-qingse", "青涩青年音色"},
	{"male-qn-jingying", "精英青年音色"},
	{"male-qn-badao", "霸道青年音色"},
	{"male-qn-daxuesheng", "青年大学生音色"},
	{"female-shaonv", "少女音色"},
	{"female-yujie", "御姐音色"},
	{"female-chengshu", "成熟女性音色"},
	{"female-tianmei", "甜美女性音色"},
	{"male-qn-qingse-jingpin", "青涩青年音色-beta"},
	{"male-qn-jingying-jingpin", "精英青年音色-beta"},
	{"male-qn-badao-jingpin", "霸道青年音色-beta"},
	{"male-qn-daxuesheng-jingpin", "青年大学生音色-beta"},
	{"female-shaonv-jingpin", "少女音色-beta"},
	{"female-yujie-jingpin", "御姐音色-beta"},
	{"female-chengshu-jingpin", "成熟女性音色-beta"},
	{"female-tianmei-jingpin", "甜美女性音色-beta"},
	{"clever_boy", "聪明男童"},
	{"cute_boy", "可爱男童"},
	{"lovely_girl", "萌萌女童"},
	{"cartoon_pig", "卡通猪小琪"},
	{"bingjiao_didi", "病娇弟弟"},
	{"junlang_nanyou", "俊朗男友"},
	{"chunzhen_xuedi", "纯真学弟"},
	{"lengdan_xiongzhang", "冷淡学长"},
	{"badao_shaoye", "霸道少爷"},
	{"tianxin_xiaoling", "甜心小玲"},
	{"qiaopi_mengmei", "俏皮萌妹"},
	{"wumei_yujie", "妩媚御姐"},
	{"diadia_xuemei", "嗲嗲学妹"},
	{"danya_xuejie", "淡雅学姐"},
	{"Chinese (Mandarin)_Reliable_Executive", "沉稳高管"},
	{"Chinese (Mandarin)_News_Anchor", "新闻女声"},
	{"Chinese (Mandarin)_Mature_Woman", "傲娇御姐"},
	{"Chinese (Mandarin)_Unrestrained_Young_Man", "不羁青年"},
	{"Arrogant_Miss", "嚣张小姐"},
	{"Robot_Armor", "机械战甲"},
	{"Chinese (Mandarin)_Kind-hearted_Antie", "热心大婶"},
	{"Chinese (Mandarin)_HK_Flight_Attendant", "港普空姐"},
	{"Chinese (Mandarin)_Humorous_Elder", "搞笑大爷"},
	{"Chinese (Mandarin)_Gentleman", "温润男声"},
	{"Chinese (Mandarin)_Warm_Bestie", "温暖闺蜜"},
	{"Chinese (Mandarin)_Male_Announcer", "播报男声"},
	{"Chinese (Mandarin)_Sweet_Lady", "甜美女声"},
	{"Chinese (Mandarin)_Southern_Young_Man", "南方小哥"},
	{"Chinese (Mandarin)_Wise_Women", "阅历姐姐"},
	{"Chinese (Mandarin)_Gentle_Youth", "温润青年"},
	{"Chinese (Mandarin)_Warm_Girl", "温暖少女"},
	{"Chinese (Mandarin)_Kind-hearted_Elder", "花甲奶奶"},
	{"Chinese (Mandarin)_Cute_Spirit", "憨憨萌兽"},
	{"Chinese (Mandarin)_Radio_Host", "电台男主播"},
	{"Chinese (Mandarin)_Lyrical_Voice", "抒情男声"},
	{"Chinese (Mandarin)_Straightforward_Boy", "率真弟弟"},
	{"Chinese (Mandarin)_Sincere_Adult", "真诚青年"},
	{"Chinese (Mandarin)_Gentle_Senior", "温柔学姐"},
	{"Chinese (Mandarin)_Stubborn_Friend", "嘴硬竹马"},
	{"Chinese (Mandarin)_Crisp_Girl", "清脆少女"},
	{"Chinese (Mandarin)_Pure-hearted_Boy", "清澈邻家弟弟"},
	{"Chinese (Mandarin)_Soft_Girl", "柔和少女"},
}

// storage 管理语音合成配置存储
type storage struct {
	sync.RWMutex
	db sql.Sqlite
}

// voiceConfig 存储语音合成配置信息
type voiceConfig struct {
	ID        int64   `db:"id"`        // 主键ID
	APIKey    string  `db:"apiKey"`    // API密钥
	APIURL    string  `db:"apiUrl"`    // API地址
	ModelName string  `db:"modelName"` // 模型名称
	VoiceID   string  `db:"voiceId"`   // 默认音色ID
	Speed     float64 `db:"speed"`     // 语速
	Volume    int     `db:"vol"`       // 音量
	Pitch     float64 `db:"pitch"`     // 音调
}

// userVoice 用户音色配置
type userVoice struct {
	UserID  int64  `db:"userId"`  // 用户ID
	VoiceID string `db:"voiceId"` // 音色ID
}

// getConfig 获取当前配置
func (sdb *storage) getConfig() voiceConfig {
	sdb.RLock()
	defer sdb.RUnlock()
	cfg := voiceConfig{}
	_ = sdb.db.Find("config", &cfg, "WHERE id = 1")
	// 返回默认配置
	if cfg.APIURL == "" {
		cfg.APIURL = DefaultAPIURL
	}
	if cfg.ModelName == "" {
		cfg.ModelName = DefaultModel
	}
	if cfg.VoiceID == "" {
		cfg.VoiceID = DefaultVoiceID
	}
	if cfg.Speed == 0 {
		cfg.Speed = DefaultSpeed
	}
	if cfg.Volume == 0 {
		cfg.Volume = DefaultVolume
	}
	if cfg.Pitch == 0 {
		cfg.Pitch = DefaultPitch
	}
	return cfg
}

// setConfig 设置语音合成配置
func (sdb *storage) setConfig(apiKey, apiURL, modelName, voiceID string, speed float64, vol int, pitch float64) error {
	sdb.Lock()
	defer sdb.Unlock()
	return sdb.db.Insert("config", &voiceConfig{
		ID:        1,
		APIKey:    apiKey,
		APIURL:    apiURL,
		ModelName: modelName,
		VoiceID:   voiceID,
		Speed:     speed,
		Volume:    vol,
		Pitch:     pitch,
	})
}

// getUserVoice 获取用户音色配置
func (sdb *storage) getUserVoice(userID int64) string {
	sdb.RLock()
	defer sdb.RUnlock()
	uv := userVoice{}
	_ = sdb.db.Find("uservoice", &uv, "WHERE userId = ?", userID)
	return uv.VoiceID
}

// setUserVoice 设置用户音色配置
func (sdb *storage) setUserVoice(userID int64, voiceID string) error {
	sdb.Lock()
	defer sdb.Unlock()
	return sdb.db.Insert("uservoice", &userVoice{
		UserID:  userID,
		VoiceID: voiceID,
	})
}

// PrintConfig 返回格式化后的配置信息
func (sdb *storage) PrintConfig() string {
	cfg := sdb.getConfig()
	var builder strings.Builder
	builder.WriteString("【当前语音合成配置】\n")
	if cfg.APIKey != "" {
		builder.WriteString(fmt.Sprintf("• 密钥: %s\n", cfg.APIKey[:8]+"***"+cfg.APIKey[len(cfg.APIKey)-4:]))
	} else {
		builder.WriteString("• 密钥: 未设置\n")
	}
	builder.WriteString(fmt.Sprintf("• 接口地址: %s\n", cfg.APIURL))
	builder.WriteString(fmt.Sprintf("• 模型名: %s\n", cfg.ModelName))
	builder.WriteString(fmt.Sprintf("• 默认音色: %s\n", cfg.VoiceID))
	builder.WriteString(fmt.Sprintf("• 语速: %.1f\n", cfg.Speed))
	builder.WriteString(fmt.Sprintf("• 音量: %d\n", cfg.Volume))
	builder.WriteString(fmt.Sprintf("• 音调: %.1f\n", cfg.Pitch))
	return builder.String()
}

// PrintVoiceList 返回音色列表
func PrintVoiceList() string {
	var builder strings.Builder
	builder.WriteString("【音色列表】回复\"我的音色+序号\"即可设置\n")
	for i, v := range VoiceList {
		builder.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, v.ID, v.Name))
	}
	return builder.String()
}

// RenderVoiceListToBase64 将音色列表渲染为图片
func RenderVoiceListToBase64() ([]byte, error) {
	return renderVoiceListToBase64Impl()
}

// renderVoiceListToBase64Impl 渲染音色列表图片的实现
func renderVoiceListToBase64Impl() ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("【音色列表】\n")
	builder.WriteString("发送\"我的音色+序号\"即可设置\n")
	builder.WriteString("─────────────────────\n")
	for i, v := range VoiceList {
		builder.WriteString(fmt.Sprintf("%2d. %s\n", i+1, v.Name))
	}
	builder.WriteString("─────────────────────\n")
	builder.WriteString(fmt.Sprintf("共 %d 个音色", len(VoiceList)))

	return text.RenderToBase64(builder.String(), text.FontFile, 400, 25)
}

// ParseVoiceInput 解析用户输入，返回音色ID
// 支持序号(1-58)、名字、ID
func ParseVoiceInput(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", false
	}

	// 尝试解析为序号
	var idx int
	_, err := fmt.Sscanf(input, "%d", &idx)
	if err == nil && idx >= 1 && idx <= len(VoiceList) {
		return VoiceList[idx-1].ID, true
	}

	// 尝试匹配ID
	for _, v := range VoiceList {
		if v.ID == input {
			return v.ID, true
		}
	}

	// 尝试匹配名字（模糊匹配）
	for _, v := range VoiceList {
		if strings.Contains(v.Name, input) {
			return v.ID, true
		}
	}

	return "", false
}
