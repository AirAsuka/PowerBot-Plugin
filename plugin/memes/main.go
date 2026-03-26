// Package memes 表情包制作 - 通过 meme-generator-rs API 生成表情包
package memes

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/FloatTech/floatbox/file"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const (
	// baseURL meme-generator-rs API 地址
	baseURL = "http://127.0.0.1:2233"
)

var (
	// keyMap 关键词 -> 表情key 的映射
	keyMap = make(map[string]string)
	// infos 表情key -> 表情信息 的映射
	infos = make(map[string]*MemeInfo)
	// dataDir 数据目录
	dataDir string
	// mu 保护 keyMap 和 infos 的读写锁
	mu sync.RWMutex
	// dataLoaded 标记数据是否已加载
	dataLoaded bool
)

func init() {
	en := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
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

	// ========== 先注册所有命令，再后台加载数据 ==========

	// 表情包列表
	en.OnFullMatchGroup([]string{"表情包列表", "meme列表", "memes列表"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text("正在生成表情包列表，请稍候..."))
			// 先确保API可达
			if !checkAPI() {
				ctx.SendChain(message.Text("ERROR: 无法连接到表情包API(", baseURL, ")"))
				return
			}
			data, err := renderMemeList()
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			b64 := base64.StdEncoding.EncodeToString(data)
			ctx.SendChain(message.Image("base64://" + b64))
		})

	// 表情包帮助
	en.OnFullMatchGroup([]string{"表情包帮助", "meme帮助", "memes帮助"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text(
				"【表情包列表】查看所有可用表情\n",
				"【{表情名称}】使用表情名称制作表情\n",
				"【{表情名称}@用户】使用被@用户的头像和昵称\n",
				"【{表情名称} 文字】附带文字制作表情\n",
				"【随机表情包】随机制作一个表情\n",
				"【表情包搜索+关键词】搜索表情包\n",
				"【表情包详情+名称】查看表情支持的参数\n",
				"【表情包更新】更新本地表情列表缓存\n",
				"Tips: 多段文字用/分隔",
			))
		})

	// 表情包更新
	en.OnFullMatchGroup([]string{"表情包更新", "meme更新", "memes更新"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text("正在更新表情包列表..."))
			// 清除缓存文件
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

	// 表情包搜索
	en.OnRegex(`^(表情包|memes?)搜索\s*(.+)$`).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ensureDataLoaded()
			keyword := ctx.State["regex_matched"].([]string)[2]
			keyword = strings.TrimSpace(keyword)
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

	// 表情包详情
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

	// 随机表情包
	en.OnFullMatchGroup([]string{"随机表情包", "随机meme", "随机memes"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ensureDataLoaded()
			mu.RLock()
			// 过滤出只需要1张图片不需要文字的表情
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
			// 使用发送者头像生成
			avatarURL := getAvatarURL(ctx.Event.UserID)
			nickname := getSenderNickname(ctx)
			handleMemeGeneration(ctx, info, avatarURL, nickname, "", nil)
		})

	// 通用表情匹配处理 - 使用 OnMessage 匹配所有消息
	en.OnMessage(func(ctx *zero.Ctx) bool {
		// 获取纯文本消息
		msg := extractPlainText(ctx)
		if msg == "" {
			return false
		}
		// 去除开头的#
		msg = strings.TrimPrefix(msg, "#")
		msg = strings.TrimSpace(msg)
		if msg == "" {
			return false
		}
		// 确保数据已加载
		ensureDataLoaded()
		mu.RLock()
		target := findLongestMatchingKey(msg, keyMap)
		mu.RUnlock()
		if target == "" {
			return false
		}
		// 保存匹配信息到 State
		ctx.State["meme_keyword"] = target
		ctx.State["meme_msg"] = msg
		return true
	}).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			target := ctx.State["meme_keyword"].(string)
			msg := ctx.State["meme_msg"].(string)

			mu.RLock()
			key := keyMap[target]
			info := infos[key]
			mu.RUnlock()

			if info == nil {
				return
			}

			// 提取关键词后面的文字
			textPart := strings.TrimPrefix(msg, target)
			textPart = strings.TrimSpace(textPart)

			// 检查是否请求详情
			if textPart == "详情" || textPart == "帮助" {
				ctx.SendChain(message.Text(formatMemeDetail(info)))
				return
			}

			// 解析@用户和图片
			atUsers := extractAtUsers(ctx)
			imgURLs := extractImageURLs(ctx)

			// 确定使用的头像URL和昵称
			var avatarURL string
			var nickname string

			if len(atUsers) > 0 {
				// 有@用户，使用被@用户的头像和昵称
				avatarURL = getAvatarURL(atUsers[0].QQ)
				nickname = atUsers[0].Nickname
				if nickname == "" {
					nickname = ctx.CardOrNickName(atUsers[0].QQ)
				}
			} else {
				// 没有@用户，使用发送者的头像和昵称
				avatarURL = getAvatarURL(ctx.Event.UserID)
				nickname = getSenderNickname(ctx)
			}

			handleMemeGeneration(ctx, info, avatarURL, nickname, textPart, imgURLs)
		})

	// ========== 后台异步加载表情数据 ==========
	go func() {
		if err := loadMemeData(); err != nil {
			logrus.Warnf("[memes] 后台加载表情列表失败: %v, 将在首次使用时重新加载", err)
		}
	}()
}

// ensureDataLoaded 确保数据已加载，如果没有则尝试加载
func ensureDataLoaded() {
	mu.RLock()
	loaded := dataLoaded
	mu.RUnlock()
	if !loaded {
		logrus.Info("[memes] 数据未加载，尝试加载...")
		_ = loadMemeData()
	}
}

// checkAPI 检查API是否可达
func checkAPI() bool {
	resp, err := httpClient.Get(baseURL + "/meme/keys")
	if err != nil {
		logrus.Warnf("[memes] API不可达: %v", err)
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// atUserInfo @用户信息
type atUserInfo struct {
	QQ       int64
	Nickname string
}

// extractAtUsers 提取消息中的@用户
func extractAtUsers(ctx *zero.Ctx) []atUserInfo {
	users := make([]atUserInfo, 0)
	for _, seg := range ctx.Event.Message {
		if seg.Type == "at" {
			qqStr := seg.Data["qq"]
			qq, err := strconv.ParseInt(qqStr, 10, 64)
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

// extractImageURLs 提取消息中的图片URL
func extractImageURLs(ctx *zero.Ctx) []string {
	urls := make([]string, 0)
	for _, seg := range ctx.Event.Message {
		if seg.Type == "image" {
			url := seg.Data["url"]
			if url != "" {
				urls = append(urls, url)
			}
		}
	}
	return urls
}

// extractPlainText 提取消息纯文本
func extractPlainText(ctx *zero.Ctx) string {
	var sb strings.Builder
	for _, seg := range ctx.Event.Message {
		if seg.Type == "text" {
			sb.WriteString(seg.Data["text"])
		}
	}
	return strings.TrimSpace(sb.String())
}

// getAvatarURL 获取用户头像URL
func getAvatarURL(qq int64) string {
	return fmt.Sprintf("https://q4.qlogo.cn/g?b=qq&nk=%d&s=640", qq)
}

// getSenderNickname 获取发送者昵称
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

// findLongestMatchingKey 查找消息开头匹配的最长关键词
func findLongestMatchingKey(msg string, km map[string]string) string {
	var longest string
	for k := range km {
		if strings.HasPrefix(msg, k) && len(k) > len(longest) {
			longest = k
		}
	}
	return longest
}

// handleMemeGeneration 处理表情生成逻辑
func handleMemeGeneration(ctx *zero.Ctx, info *MemeInfo, defaultAvatarURL string, nickname string, textPart string, imgURLs []string) {
	// 获取@用户列表（可能有多个）
	atUsers := extractAtUsers(ctx)

	// 构建图片列表
	imageIDs := make([]MemeImage, 0)

	if info.Params.MaxImages > 0 {
		// 收集所有需要的图片URL
		allImgURLs := make([]string, 0)

		if len(imgURLs) > 0 {
			// 消息中带了图片
			allImgURLs = append(allImgURLs, imgURLs...)
		} else if len(atUsers) > 0 {
			// 有@用户，使用被@用户的头像
			for _, u := range atUsers {
				allImgURLs = append(allImgURLs, getAvatarURL(u.QQ))
			}
		}

		// 如果没有图片，使用默认头像（发送者自己）
		if len(allImgURLs) == 0 {
			allImgURLs = append(allImgURLs, defaultAvatarURL)
		}

		// 如果图片不够最小要求，补上发送者头像到最前面
		if len(allImgURLs) < info.Params.MinImages {
			senderURL := getAvatarURL(ctx.Event.UserID)
			// 检查是否已经包含发送者头像
			hasSender := false
			for _, u := range allImgURLs {
				if u == senderURL {
					hasSender = true
					break
				}
			}
			if !hasSender {
				allImgURLs = append([]string{senderURL}, allImgURLs...)
			}
		}

		// 截取到最大图片数
		if len(allImgURLs) > info.Params.MaxImages {
			allImgURLs = allImgURLs[:info.Params.MaxImages]
		}

		// 上传图片
		for i, imgURL := range allImgURLs {
			logrus.Infof("[memes] 上传第%d张图片: %s", i+1, imgURL)
			imageID, err := uploadImageByURL(imgURL)
			if err != nil {
				logrus.Warnf("[memes] URL上传失败: %v, 尝试下载后base64上传", err)
				// URL上传失败，尝试先下载再通过base64上传
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
			imageIDs = append(imageIDs, MemeImage{
				Name: fmt.Sprintf("image_%d", i),
				ID:   imageID,
			})
		}
	}

	// 处理文字
	texts := make([]string, 0)
	if textPart != "" && info.Params.MaxTexts > 0 {
		// 用/分隔多段文字
		parts := strings.SplitN(textPart, "/", info.Params.MaxTexts)
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				texts = append(texts, p)
			}
		}
	}

	// 如果需要文字但没有提供，使用昵称
	if len(texts) == 0 && info.Params.MinTexts > 0 {
		if len(atUsers) > 0 {
			// 使用@用户的昵称
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
		// 还是不够就用发送者昵称
		if len(texts) < info.Params.MinTexts {
			texts = append(texts, nickname)
		}
	}

	// 文字太少校验
	if len(texts) < info.Params.MinTexts {
		ctx.SendChain(message.Text(fmt.Sprintf("需要至少%d段文字，请用/分隔！", info.Params.MinTexts)))
		return
	}

	// 截取文字到最大数量
	if len(texts) > info.Params.MaxTexts && info.Params.MaxTexts > 0 {
		texts = texts[:info.Params.MaxTexts]
	}

	// 生成表情
	data, err := generateMeme(info.Key, imageIDs, texts, nil)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: 生成表情失败: ", err))
		return
	}

	// 发送结果
	b64 := base64.StdEncoding.EncodeToString(data)
	ctx.SendChain(message.Image("base64://" + b64))
}

// httpGetBytes 通过HTTP GET获取数据
func httpGetBytes(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// formatMemeDetail 格式化表情详情
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

// loadMemeData 加载表情数据（优先从本地缓存，否则从API获取）
func loadMemeData() error {
	infosPath := dataDir + "/infos.json"
	keymapPath := dataDir + "/keymap.json"

	var localInfos map[string]*MemeInfo
	var localKeyMap map[string]string

	// 尝试从本地缓存读取
	if file.IsExist(infosPath) && file.IsExist(keymapPath) {
		infosData, err := os.ReadFile(infosPath)
		if err == nil {
			keymapData, err := os.ReadFile(keymapPath)
			if err == nil {
				err1 := json.Unmarshal(infosData, &localInfos)
				err2 := json.Unmarshal(keymapData, &localKeyMap)
				if err1 == nil && err2 == nil && len(localInfos) > 0 && len(localKeyMap) > 0 {
					mu.Lock()
					infos = localInfos
					keyMap = localKeyMap
					dataLoaded = true
					mu.Unlock()
					logrus.Infof("[memes] 从本地缓存加载了 %d 个表情", len(localInfos))
					return nil
				}
			}
		}
	}

	// 从API获取
	logrus.Info("[memes] 正在从API获取表情列表...")
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
	dataLoaded = true
	mu.Unlock()

	// 保存到本地缓存
	infosJSON, err := json.Marshal(newInfos)
	if err == nil {
		_ = os.WriteFile(infosPath, infosJSON, 0644)
	}
	keymapJSON, err := json.Marshal(newKeyMap)
	if err == nil {
		_ = os.WriteFile(keymapPath, keymapJSON, 0644)
	}

	logrus.Infof("[memes] 从API加载了 %d 个表情, %d 个关键词", len(newInfos), len(newKeyMap))
	return nil
}
