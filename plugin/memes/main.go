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
	logrus.Info("[memes] ========== 插件初始化开始 ==========")

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
	logrus.Info("[memes] control.AutoRegister 完成")

	dataDir = file.BOTPATH + "/" + en.DataFolder()
	_ = os.MkdirAll(dataDir, 0755)
	logrus.Infof("[memes] 数据目录: %s", dataDir)

	// ========== 先注册所有命令 ==========

	// 表情包列表
	en.OnFullMatchGroup([]string{"表情包列表", "meme列表", "memes列表"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			logrus.Info("[memes] >>> 触发: 表情包列表")
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
			b64 := base64.StdEncoding.EncodeToString(data)
			ctx.SendChain(message.Image("base64://" + b64))
		})
	logrus.Info("[memes] 已注册: 表情包列表")

	// 表情包帮助
	en.OnFullMatchGroup([]string{"表情包帮助", "meme帮助", "memes帮助"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			logrus.Info("[memes] >>> 触发: 表情包帮助")
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
	logrus.Info("[memes] 已注册: 表情包帮助")

	// 表情包更新
	en.OnFullMatchGroup([]string{"表情包更新", "meme更新", "memes更新"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			logrus.Info("[memes] >>> 触发: 表情包更新")
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
	logrus.Info("[memes] 已注册: 表情包更新")

	// 表情包搜索
	en.OnRegex(`^(表情包|memes?)搜索\s*(.+)$`).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			logrus.Info("[memes] >>> 触发: 表情包搜索")
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
	logrus.Info("[memes] 已注册: 表情包搜索")

	// 表情包详情
	en.OnRegex(`^(表情包|memes?)详情\s*(.+)$`).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			logrus.Info("[memes] >>> 触发: 表情包详情")
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
	logrus.Info("[memes] 已注册: 表情包详情")

	// 随机表情包
	en.OnFullMatchGroup([]string{"随机表情包", "随机meme", "随机memes"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			logrus.Info("[memes] >>> 触发: 随机表情包")
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
			avatarURL := getAvatarURL(ctx.Event.UserID)
			nickname := getSenderNickname(ctx)
			handleMemeGeneration(ctx, info, avatarURL, nickname, "", nil)
		})
	logrus.Info("[memes] 已注册: 随机表情包")

	// 通用表情匹配处理
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
		target := findLongestMatchingKey(msg, keyMap)
		mu.RUnlock()
		if target == "" {
			return false
		}
		logrus.Infof("[memes] OnMessage匹配成功: msg=%q keyword=%q", msg, target)
		ctx.State["meme_keyword"] = target
		ctx.State["meme_msg"] = msg
		return true
	}).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			target := ctx.State["meme_keyword"].(string)
			msg := ctx.State["meme_msg"].(string)
			logrus.Infof("[memes] >>> 触发: 表情生成 keyword=%q msg=%q", target, msg)

			mu.RLock()
			key := keyMap[target]
			info := infos[key]
			mu.RUnlock()

			if info == nil {
				logrus.Warnf("[memes] 未找到表情info: key=%q", key)
				return
			}

			textPart := strings.TrimPrefix(msg, target)
			textPart = strings.TrimSpace(textPart)

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
	logrus.Info("[memes] 已注册: 通用表情匹配(OnMessage)")

	// ========== 同时注册一个不经过 control 的全局测试handler ==========
	zero.OnFullMatch("memes测试", zero.OnlyToMe).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		logrus.Info("[memes] >>> 触发: memes测试(全局)")
		ctx.SendChain(message.Text("[memes] 插件已加载，测试通过！"))
	})
	logrus.Info("[memes] 已注册: memes测试(全局, 不经过control)")

	logrus.Info("[memes] ========== 所有命令注册完成，开始后台加载数据 ==========")

	// ========== 后台异步加载表情数据 ==========
	go func() {
		logrus.Info("[memes] 后台goroutine: 开始加载表情数据")
		if err := loadMemeData(); err != nil {
			logrus.Warnf("[memes] 后台加载表情列表失败: %v", err)
		} else {
			mu.RLock()
			logrus.Infof("[memes] 后台加载完成: %d 个表情, %d 个关键词", len(infos), len(keyMap))
			mu.RUnlock()
		}
	}()

	logrus.Info("[memes] ========== 插件初始化结束 ==========")
}

// ensureDataLoaded 确保数据已加载
func ensureDataLoaded() {
	mu.RLock()
	loaded := dataLoaded
	mu.RUnlock()
	if !loaded {
		logrus.Info("[memes] ensureDataLoaded: 数据未加载，尝试加载...")
		if err := loadMemeData(); err != nil {
			logrus.Warnf("[memes] ensureDataLoaded: 加载失败: %v", err)
		}
	}
}

// checkAPI 检查API是否可达
func checkAPI() bool {
	logrus.Infof("[memes] checkAPI: 检查 %s", baseURL)
	resp, err := httpClient.Get(baseURL + "/meme/keys")
	if err != nil {
		logrus.Warnf("[memes] checkAPI: API不可达: %v", err)
		return false
	}
	resp.Body.Close()
	logrus.Infof("[memes] checkAPI: 状态码=%d", resp.StatusCode)
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
	logrus.Infof("[memes] handleMemeGeneration: key=%s avatar=%s nick=%s text=%q imgs=%d",
		info.Key, defaultAvatarURL, nickname, textPart, len(imgURLs))

	atUsers := extractAtUsers(ctx)
	imageIDs := make([]MemeImage, 0)

	if info.Params.MaxImages > 0 {
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

		// 如果图片不够最小要求，补上发送者头像到末尾
		// meme-generator-rs 约定: images[0]=目标(被操作者), images[1]=自己(操作者)
		// 当 A @B 时: images=[B的头像, A的头像]，B是目标，A是操作者
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
				allImgURLs = append(allImgURLs, senderURL) // 追加到末尾，作为操作者
			}
		}

		if len(allImgURLs) > info.Params.MaxImages {
			allImgURLs = allImgURLs[:info.Params.MaxImages]
		}

		logrus.Infof("[memes] 需要上传 %d 张图片", len(allImgURLs))

		for i, imgURL := range allImgURLs {
			logrus.Infof("[memes] 上传第%d张图片: %s", i+1, imgURL)
			imageID, err := uploadImageByURL(imgURL)
			if err != nil {
				logrus.Warnf("[memes] URL上传失败: %v, 尝试下载后base64上传", err)
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

	if len(texts) < info.Params.MinTexts {
		ctx.SendChain(message.Text(fmt.Sprintf("需要至少%d段文字，请用/分隔！", info.Params.MinTexts)))
		return
	}

	if len(texts) > info.Params.MaxTexts && info.Params.MaxTexts > 0 {
		texts = texts[:info.Params.MaxTexts]
	}

	logrus.Infof("[memes] 调用generateMeme: key=%s images=%d texts=%v", info.Key, len(imageIDs), texts)
	data, err := generateMeme(info.Key, imageIDs, texts, nil)
	if err != nil {
		logrus.Errorf("[memes] 生成表情失败: %v", err)
		ctx.SendChain(message.Text("ERROR: 生成表情失败: ", err))
		return
	}

	logrus.Infof("[memes] 生成成功, 数据大小=%d bytes, 发送中...", len(data))
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

// loadMemeData 加载表情数据
func loadMemeData() error {
	infosPath := dataDir + "/infos.json"
	keymapPath := dataDir + "/keymap.json"
	logrus.Infof("[memes] loadMemeData: infosPath=%s keymapPath=%s", infosPath, keymapPath)

	var localInfos map[string]*MemeInfo
	var localKeyMap map[string]string

	// 尝试从本地缓存读取
	if file.IsExist(infosPath) && file.IsExist(keymapPath) {
		logrus.Info("[memes] loadMemeData: 发现本地缓存文件，尝试读取")
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
					logrus.Infof("[memes] loadMemeData: 从本地缓存加载成功: %d 个表情, %d 个关键词", len(localInfos), len(localKeyMap))
					return nil
				}
				logrus.Warnf("[memes] loadMemeData: 缓存解析失败: err1=%v err2=%v infos=%d keymap=%d",
					err1, err2, len(localInfos), len(localKeyMap))
			} else {
				logrus.Warnf("[memes] loadMemeData: 读取keymap缓存失败: %v", err)
			}
		} else {
			logrus.Warnf("[memes] loadMemeData: 读取infos缓存失败: %v", err)
		}
	} else {
		logrus.Info("[memes] loadMemeData: 本地缓存文件不存在")
	}

	// 从API获取
	logrus.Infof("[memes] loadMemeData: 从API获取表情列表 %s/meme/infos ...", baseURL)
	memeInfos, err := fetchMemeInfos()
	if err != nil {
		logrus.Errorf("[memes] loadMemeData: API获取失败: %v", err)
		return fmt.Errorf("从API获取表情列表失败: %w", err)
	}
	logrus.Infof("[memes] loadMemeData: API返回 %d 个表情", len(memeInfos))

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

	logrus.Infof("[memes] loadMemeData: 加载完成并已缓存, %d 个表情, %d 个关键词", len(newInfos), len(newKeyMap))
	return nil
}
