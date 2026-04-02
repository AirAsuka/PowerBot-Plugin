package memes

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/FloatTech/floatbox/file"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const (
	baseURL = "http://127.0.0.1:2233"
)

var (
	en *control.Engine

	keyMap     = make(map[string]string)
	patternMap = make(map[*regexp.Regexp]string)
	infos      = make(map[string]*MemeInfo)
	dataDir    string
	mu         sync.RWMutex
	loaded     bool
)

func init() {
	en = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "表情包制作",
		Help: "- 表情包列表 (查看所有可用表情)\n" +
			"- 表情包搜索XXX (搜索表情关键词)\n" +
			"- 表情包帮助 (查看使用帮助)\n" +
			"- 表情包详情XXX (查看表情参数详情)\n" +
			"- 表情包更新 (更新表情列表缓存)\n" +
			"- 随机表情包 (随机生成一个表情)\n" +
			"- {表情关键词}[@用户] (制作表情)\n" +
			"Tips: 使用表情关键词时可@用户使用对方头像，不@则用自己头像",
		PrivateDataFolder: "memes",
	})

	dataDir = file.BOTPATH + "/" + en.DataFolder()
	_ = os.MkdirAll(dataDir, 0755)

	registerCommands()
	go loadMemeData()

	en.OnFullMatch("memes测试").SetBlock(true).Handle(func(ctx *zero.Ctx) {
		mu.RLock()
		kmLen := len(keyMap)
		infosLen := len(infos)
		mu.RUnlock()
		ctx.SendChain(message.Text(fmt.Sprintf("memes插件已加载！keyMap=%d, infos=%d", kmLen, infosLen)))
	})
}

func registerCommands() {
	en.OnFullMatchGroup([]string{"表情包列表", "meme列表", "memes列表"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text("正在生成表情包列表，请稍候..."))
			if !checkAPI() {
				ctx.SendChain(message.Text("ERROR: 无法连接到表情包API(", baseURL, ")"))
				return
			}
			data, err := renderMemeList()
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Image("base64://" + base64.StdEncoding.EncodeToString(data)))
		})

	en.OnFullMatchGroup([]string{"表情包帮助", "meme帮助", "memes帮助"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text(
				"【表情包列表】查看所有可用表情\n" +
					"【{表情名称}】使用表情名称制作表情\n" +
					"【{表情名称}@用户】使用被@用户的头像和昵称\n" +
					"【{表情名称} 文字】附带文字制作表情\n" +
					"【随机表情包】随机制作一个表情\n" +
					"【表情包搜索+关键词】搜索表情包\n" +
					"【表情包详情+名称】查看表情支持的参数\n" +
					"【表情包更新】更新本地表情列表缓存\n" +
					"Tips: 多段文字用/分隔",
			))
		})

	en.OnFullMatchGroup([]string{"表情包更新", "meme更新", "memes更新"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text("正在更新表情包列表..."))
			_ = os.Remove(dataDir + "/infos.json")
			_ = os.Remove(dataDir + "/keymap.json")
			err := loadMemeData()
			if err != nil {
				ctx.SendChain(message.Text("ERROR: 更新失败: ", err))
				return
			}
			mu.RLock()
			count := len(infos)
			mu.RUnlock()
			ctx.SendChain(message.Text(fmt.Sprintf("更新完成！共加载 %d 个表情", count)))
		})

	en.OnRegex(`^(表情包|memes?)搜索\s*(.+)$`).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ensureDataLoaded()
			keyword := strings.TrimSpace(ctx.State["regex_matched"].([]string)[2])
			if keyword == "" {
				ctx.SendChain(message.Text("请输入搜索关键词"))
				return
			}
			mu.RLock()
			hits := make([]string, 0)
			for k := range keyMap {
				if strings.Contains(k, keyword) {
					hits = append(hits, k)
				}
			}
			mu.RUnlock()
			sort.Strings(hits)
			if len(hits) == 0 {
				ctx.SendChain(message.Text("未找到包含\"", keyword, "\"的表情"))
				return
			}
			result := "搜索结果："
			for i, h := range hits {
				if i >= 30 {
					result += fmt.Sprintf("\n... 等共%d个结果", len(hits))
					break
				}
				result += fmt.Sprintf("\n%d. %s", i+1, h)
			}
			ctx.SendChain(message.Text(result))
		})

	en.OnRegex(`^(表情包|memes?)详情\s*(.+)$`).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ensureDataLoaded()
			name := strings.TrimSpace(ctx.State["regex_matched"].([]string)[2])
			mu.RLock()
			key, ok := keyMap[name]
			var info *MemeInfo
			if ok {
				info = infos[key]
			}
			mu.RUnlock()
			if !ok || info == nil {
				ctx.SendChain(message.Text("未找到表情：", name))
				return
			}
			ctx.SendChain(message.Text(formatMemeDetail(info)))
		})

	en.OnFullMatchGroup([]string{"随机表情包", "随机meme", "随机memes"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ensureDataLoaded()
			mu.RLock()
			candidates := make([]*MemeInfo, 0)
			for _, info := range infos {
				if info.Params.MinImages <= 1 && info.Params.MaxImages >= 1 && info.Params.MinTexts == 0 {
					candidates = append(candidates, info)
				}
			}
			mu.RUnlock()
			if len(candidates) == 0 {
				ctx.SendChain(message.Text("暂无可用表情，请先发送\"表情包更新\""))
				return
			}
			info := candidates[rand.Intn(len(candidates))]
			handleMemeGeneration(ctx, info, getAvatarURL(ctx.Event.UserID), getSenderNickname(ctx), "", nil)
		})

	en.OnMessage(func(ctx *zero.Ctx) bool {
		msg := extractPlainText(ctx)
		if msg == "" {
			return false
		}
		msg = strings.TrimPrefix(msg, "#")
		msg = strings.TrimSpace(msg)
		if msg == "" {
			return false
		}
		ensureDataLoaded()

		mu.RLock()
		matched := findMatchingMeme(msg)
		mu.RUnlock()

		if matched == nil {
			return false
		}

		ctx.State["meme_info"] = matched.info
		ctx.State["meme_keyword"] = matched.keyword
		ctx.State["meme_msg"] = msg
		ctx.State["meme_text_part"] = matched.textPart
		return true
	}).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			info := ctx.State["meme_info"].(*MemeInfo)
			_ = ctx.State["meme_keyword"].(string)
			_ = ctx.State["meme_msg"].(string)
			textPart := ctx.State["meme_text_part"].(string)

			if textPart == "详情" || textPart == "帮助" {
				ctx.SendChain(message.Text(formatMemeDetail(info)))
				return
			}

			atUsers := extractAtUsers(ctx)
			imgURLs := extractImageURLs(ctx)

			var avatarURL string
			var nickname string

			if len(atUsers) > 0 {
				avatarURL = getAvatarURL(atUsers[0].QQ)
				nickname = atUsers[0].Nickname
				if nickname == "" {
					nickname = ctx.CardOrNickName(atUsers[0].QQ)
				}
			} else {
				avatarURL = getAvatarURL(ctx.Event.UserID)
				nickname = getSenderNickname(ctx)
			}

			handleMemeGeneration(ctx, info, avatarURL, nickname, textPart, imgURLs)
		})
}

type matchedMeme struct {
	info     *MemeInfo
	keyword  string
	textPart string
}

func findMatchingMeme(msg string) *matchedMeme {
	var longest *matchedMeme

	for keyword, memeKey := range keyMap {
		if strings.HasPrefix(msg, keyword) {
			remainder := msg[len(keyword):]

			if remainder == "" || strings.HasPrefix(remainder, " ") ||
				strings.HasPrefix(remainder, "@") || strings.HasPrefix(remainder, "[CQ:") {

				mInfo := infos[memeKey]
				if mInfo == nil {
					continue
				}
				textPart := strings.TrimSpace(remainder)

				if longest == nil || len(keyword) > len(longest.keyword) {
					longest = &matchedMeme{
						info:     mInfo,
						keyword:  keyword,
						textPart: textPart,
					}
				}
			}
		}
	}

	return longest
}

func ensureDataLoaded() {
	mu.RLock()
	if loaded {
		mu.RUnlock()
		return
	}
	mu.RUnlock()
	loadMemeData()
}

func checkAPI() bool {
	resp, err := httpClient.Get(baseURL + "/meme/keys")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type atUserInfo struct {
	QQ       int64
	Nickname string
}

func extractAtUsers(ctx *zero.Ctx) []atUserInfo {
	users := make([]atUserInfo, 0)
	for _, seg := range ctx.Event.Message {
		if seg.Type == "at" {
			qq, err := strconv.ParseInt(seg.Data["qq"], 10, 64)
			if err != nil {
				continue
			}
			nick := seg.Data["name"]
			if nick == "" {
				nick = ctx.CardOrNickName(qq)
			}
			users = append(users, atUserInfo{QQ: qq, Nickname: nick})
		}
	}
	return users
}

func extractImageURLs(ctx *zero.Ctx) []string {
	urls := make([]string, 0)
	for _, seg := range ctx.Event.Message {
		if seg.Type == "image" {
			if url := seg.Data["url"]; url != "" {
				urls = append(urls, url)
			}
		}
	}
	return urls
}

func extractPlainText(ctx *zero.Ctx) string {
	var sb strings.Builder
	for _, seg := range ctx.Event.Message {
		if seg.Type == "text" {
			sb.WriteString(seg.Data["text"])
		}
	}
	return strings.TrimSpace(sb.String())
}

func extractReplyID(ctx *zero.Ctx) int64 {
	for _, seg := range ctx.Event.Message {
		if seg.Type == "reply" {
			if id, ok := seg.Data["id"]; ok {
				replyID, _ := strconv.ParseInt(id, 10, 64)
				return replyID
			}
		}
	}
	return 0
}

func getAvatarURL(qq int64) string {
	return fmt.Sprintf("https://q4.qlogo.cn/g?b=qq&nk=%d&s=640", qq)
}

func getSenderNickname(ctx *zero.Ctx) string {
	name := ctx.CardOrNickName(ctx.Event.UserID)
	if name == "" {
		name = ctx.Event.Sender.NickName
	}
	if name == "" {
		name = strconv.FormatInt(ctx.Event.UserID, 10)
	}
	return name
}

func handleMemeGeneration(ctx *zero.Ctx, info *MemeInfo, defaultAvatarURL string, nickname string, textPart string, imgURLs []string) {
	atUsers := extractAtUsers(ctx)
	imageIDs := make([]MemeImage, 0)
	allImgURLs := make([]string, 0)

	if len(imgURLs) > 0 {
		allImgURLs = append(allImgURLs, imgURLs...)
	} else if len(atUsers) > 0 {
		for _, u := range atUsers {
			allImgURLs = append(allImgURLs, getAvatarURL(u.QQ))
		}
	}

	if len(allImgURLs) == 0 {
		allImgURLs = append(allImgURLs, defaultAvatarURL)
	}

	if len(allImgURLs) < info.Params.MinImages {
		senderURL := getAvatarURL(ctx.Event.UserID)
		hasSender := false
		for _, u := range allImgURLs {
			if u == senderURL {
				hasSender = true
				break
			}
		}
		if !hasSender {
			allImgURLs = append(allImgURLs, senderURL)
		}
	}

	if len(allImgURLs) > info.Params.MaxImages {
		allImgURLs = allImgURLs[:info.Params.MaxImages]
	}

	if info.Params.MaxImages > 0 {
		if len(allImgURLs) < info.Params.MinImages {
			if info.Params.MinImages == info.Params.MaxImages {
				ctx.SendChain(message.Text(fmt.Sprintf("图片数量不符，需要 %d 张图片", info.Params.MinImages)))
			} else {
				ctx.SendChain(message.Text(fmt.Sprintf("图片数量不符，需要 %d ~ %d 张图片",
					info.Params.MinImages, info.Params.MaxImages)))
			}
			return
		}

		for _, imgURL := range allImgURLs {
			imageID, err := uploadImageByURL(imgURL)
			if err != nil {
				imgData, dlErr := httpGetBytes(imgURL)
				if dlErr != nil {
					ctx.SendChain(message.Text("ERROR: 获取图片失败: ", dlErr))
					return
				}
				imageID, err = uploadImageByBase64(imgData)
				if err != nil {
					ctx.SendChain(message.Text("ERROR: 上传图片失败: ", err))
					return
				}
			}
			imageIDs = append(imageIDs, MemeImage{Name: "image_" + strconv.Itoa(len(imageIDs)), ID: imageID})
		}
	}

	texts := make([]string, 0)
	if textPart != "" && info.Params.MaxTexts > 0 {
		parts := strings.SplitN(textPart, "/", info.Params.MaxTexts)
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				texts = append(texts, p)
			}
		}
	}

	if len(texts) == 0 && info.Params.MinTexts > 0 {
		if len(atUsers) > 0 {
			for _, u := range atUsers {
				nick := u.Nickname
				if nick == "" {
					nick = ctx.CardOrNickName(u.QQ)
				}
				texts = append(texts, nick)
				if len(texts) >= info.Params.MinTexts {
					break
				}
			}
		}
		if len(texts) < info.Params.MinTexts {
			texts = append(texts, nickname)
		}
	}

	if info.Params.MinTexts > 0 && len(texts) < info.Params.MinTexts {
		if info.Params.MaxTexts == info.Params.MinTexts {
			ctx.SendChain(message.Text(fmt.Sprintf("需要 %d 段文字，请用 / 分隔！", info.Params.MinTexts)))
		} else {
			ctx.SendChain(message.Text(fmt.Sprintf("需要 %d ~ %d 段文字，请用 / 分隔！",
				info.Params.MinTexts, info.Params.MaxTexts)))
		}
		return
	}

	if info.Params.MaxTexts > 0 && len(texts) > info.Params.MaxTexts {
		texts = texts[:info.Params.MaxTexts]
	}

	data, err := generateMeme(info.Key, imageIDs, texts, nil)
	if err != nil {
		if apiErr, ok := err.(*MemeAPIError); ok {
			ctx.SendChain(message.Text(apiErr.UserMessage()))
		} else {
			ctx.SendChain(message.Text("ERROR: 生成表情失败: ", err))
		}
		return
	}

	ctx.SendChain(message.Image("base64://" + base64.StdEncoding.EncodeToString(data)))
}

func httpGetBytes(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func formatMemeDetail(info *MemeInfo) string {
	keywords := strings.Join(info.Keywords, "、")
	detail := fmt.Sprintf(
		"【代码】%s\n【名称】%s\n【最大图片数量】%d\n【最小图片数量】%d\n【最大文本数量】%d\n【最小文本数量】%d",
		info.Key, keywords,
		info.Params.MaxImages, info.Params.MinImages,
		info.Params.MaxTexts, info.Params.MinTexts,
	)
	if len(info.Params.DefaultTexts) > 0 {
		detail += "\n【默认文本】" + strings.Join(info.Params.DefaultTexts, "/")
	}
	return detail
}

func loadMemeData() error {
	infosPath := dataDir + "/infos.json"
	keymapPath := dataDir + "/keymap.json"

	var localInfos map[string]*MemeInfo
	var localKeyMap map[string]string

	if file.IsExist(infosPath) && file.IsExist(keymapPath) {
		infosData, err := os.ReadFile(infosPath)
		if err == nil {
			keymapData, err := os.ReadFile(keymapPath)
			if err == nil {
				_ = json.Unmarshal(infosData, &localInfos)
				_ = json.Unmarshal(keymapData, &localKeyMap)
				if len(localInfos) > 0 && len(localKeyMap) > 0 {
					mu.Lock()
					infos = localInfos
					keyMap = localKeyMap
					loaded = true
					mu.Unlock()
					compilePatterns()
					return nil
				}
			}
		}
	}

	memeInfos, err := fetchMemeInfos()
	if err != nil {
		return fmt.Errorf("从API获取表情列表失败: %w", err)
	}

	newInfos := make(map[string]*MemeInfo)
	newKeyMap := make(map[string]string)
	for i := range memeInfos {
		info := &memeInfos[i]
		newInfos[info.Key] = info
		for _, kw := range info.Keywords {
			newKeyMap[kw] = info.Key
		}
	}

	mu.Lock()
	infos = newInfos
	keyMap = newKeyMap
	loaded = true
	mu.Unlock()

	_ = os.WriteFile(infosPath, mustJSON(newInfos), 0644)
	_ = os.WriteFile(keymapPath, mustJSON(newKeyMap), 0644)

	compilePatterns()
	return nil
}

func compilePatterns() {
	patternMap = make(map[*regexp.Regexp]string)
	for key, info := range infos {
		for _, pattern := range info.Patterns {
			if re, err := regexp.Compile(pattern); err == nil {
				patternMap[re] = key
			}
		}
	}
}

func mustJSON(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}
