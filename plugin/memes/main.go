// Package memes 表情包制作 - 连接本地部署的 meme-generator-rs
package memes

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/FloatTech/floatbox/web"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var (
	// memeInfoMap key -> MemeInfo 的映射
	memeInfoMap = make(map[string]*MemeInfo)
	// keywordMap keyword -> MemeInfo 的映射（用于通过关键词查找表情）
	keywordMap = make(map[string]*MemeInfo)
	// shortcutMap 快捷指令正则 -> (MemeInfo, MemeShortcut) 的映射
	shortcutMap     = make(map[string]*shortcutEntry)
	shortcutRegexps []*shortcutMatcher

	mu sync.RWMutex
)

type shortcutEntry struct {
	info     *MemeInfo
	shortcut MemeShortcut
}

type shortcutMatcher struct {
	re       *regexp.Regexp
	info     *MemeInfo
	shortcut MemeShortcut
}

func init() {
	en := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "表情包制作",
		Help: "- 表情包制作 / 表情列表\n" +
			"- 表情详情 [关键词]\n" +
			"- 表情搜索 [关键词]\n" +
			"- 表情预览 [关键词]\n" +
			"- [关键词] [图片/@某人/文字]\n" +
			"- 随机表情 [图片/@某人/文字]\n" +
			"- 更新表情\n" +
			"Tips: 触发方式为 关键词+图片/文字/@某人",
	})

	// 初始化加载表情列表
	loadMemes()

	// 表情列表命令
	en.OnFullMatchGroup([]string{"表情包制作", "表情列表", "表情包列表"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text("正在生成表情列表..."))
			imgData, err := renderMemeList()
			if err != nil {
				ctx.SendChain(message.Text("生成表情列表失败: ", err))
				return
			}
			b64 := base64.StdEncoding.EncodeToString(imgData)
			ctx.SendChain(message.Image("base64://" + b64))
		})

	// 表情详情命令
	en.OnPrefix("表情详情").SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			name := strings.TrimSpace(ctx.State["args"].(string))
			if name == "" {
				ctx.SendChain(message.Text("请输入表情关键词"))
				return
			}
			info := findMemeInfo(name)
			if info == nil {
				// 尝试搜索
				keys, err := searchMemes(name, true)
				if err == nil && len(keys) > 0 {
					result := "未找到表情「" + name + "」，你可能在找：\n"
					for i, k := range keys {
						if i >= 5 {
							break
						}
						if mi, ok := memeInfoMap[k]; ok {
							result += fmt.Sprintf("  %d. %s (%s)\n", i+1, k, strings.Join(mi.Keywords, "/"))
						} else {
							result += fmt.Sprintf("  %d. %s\n", i+1, k)
						}
					}
					ctx.SendChain(message.Text(result))
				} else {
					ctx.SendChain(message.Text("未找到表情「", name, "」"))
				}
				return
			}
			params := info.Params
			keywords := strings.Join(info.Keywords, "、")

			imageNum := strconv.Itoa(params.MinImages)
			if params.MaxImages > params.MinImages {
				imageNum += " ~ " + strconv.Itoa(params.MaxImages)
			}
			textNum := strconv.Itoa(params.MinTexts)
			if params.MaxTexts > params.MinTexts {
				textNum += " ~ " + strconv.Itoa(params.MaxTexts)
			}

			text := fmt.Sprintf("表情名：%s\n关键词：%s\n需要图片数目：%s\n需要文字数目：%s",
				info.Key, keywords, imageNum, textNum)

			if len(info.Tags) > 0 {
				text += "\n标签：" + strings.Join(info.Tags, "、")
			}
			if len(params.DefaultTexts) > 0 {
				text += "\n默认文字：[" + strings.Join(params.DefaultTexts, ", ") + "]"
			}

			msgs := []message.Segment{message.Text(text)}

			// 尝试生成预览
			preview, err := generateMemePreview(info.Key)
			if err == nil && len(preview) > 0 {
				b64 := base64.StdEncoding.EncodeToString(preview)
				msgs = append(msgs, message.Text("\n表情预览：\n"), message.Image("base64://"+b64))
			}
			ctx.SendChain(msgs...)
		})

	// 表情预览命令
	en.OnPrefix("表情预览").SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			name := strings.TrimSpace(ctx.State["args"].(string))
			if name == "" {
				ctx.SendChain(message.Text("请输入表情关键词"))
				return
			}
			info := findMemeInfo(name)
			if info == nil {
				ctx.SendChain(message.Text("未找到表情「", name, "」"))
				return
			}
			preview, err := generateMemePreview(info.Key)
			if err != nil {
				ctx.SendChain(message.Text("生成预览失败: ", err))
				return
			}
			b64 := base64.StdEncoding.EncodeToString(preview)
			ctx.SendChain(message.Image("base64://" + b64))
		})

	// 表情搜索命令
	en.OnPrefix("表情搜索").SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			name := strings.TrimSpace(ctx.State["args"].(string))
			if name == "" {
				ctx.SendChain(message.Text("请输入搜索关键词"))
				return
			}
			keys, err := searchMemes(name, true)
			if err != nil {
				ctx.SendChain(message.Text("搜索出错: ", err))
				return
			}
			if len(keys) == 0 {
				ctx.SendChain(message.Text("没有找到相关表情"))
				return
			}
			result := "搜索结果：\n"
			for i, k := range keys {
				if i >= 20 {
					result += fmt.Sprintf("... 共 %d 条结果\n", len(keys))
					break
				}
				mu.RLock()
				mi, ok := memeInfoMap[k]
				mu.RUnlock()
				if ok {
					result += fmt.Sprintf("  %d. %s (%s)\n", i+1, k, strings.Join(mi.Keywords, "/"))
				} else {
					result += fmt.Sprintf("  %d. %s\n", i+1, k)
				}
			}
			ctx.SendChain(message.Text(result))
		})

	// 更新表情命令
	en.OnFullMatchGroup([]string{"更新表情", "刷新表情"}).
		SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text("正在更新表情列表..."))
			loadMemes()
			ver, err := getVersion()
			if err != nil {
				ver = "未知"
			}
			mu.RLock()
			count := len(memeInfoMap)
			mu.RUnlock()
			ctx.SendChain(message.Text(fmt.Sprintf("表情更新成功！\n版本：%s\n共 %d 个表情", ver, count)))
		})

	// 随机表情命令
	en.OnPrefix("随机表情").SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			images, texts := extractParams(ctx)
			mu.RLock()
			var available []*MemeInfo
			for _, info := range memeInfoMap {
				if info.Params.MinImages <= len(images) && len(images) <= info.Params.MaxImages &&
					info.Params.MinTexts <= len(texts) && len(texts) <= info.Params.MaxTexts {
					available = append(available, info)
				}
			}
			mu.RUnlock()
			if len(available) == 0 {
				ctx.SendChain(message.Text("找不到符合参数数量的表情"))
				return
			}
			chosen := available[rand.Intn(len(available))]
			doGenerate(ctx, chosen, images, texts, nil)
		})

	// 关键词触发表情生成（核心功能）
	// 使用 OnMessage 低优先级匹配，检查消息是否以某个表情关键词开头
	en.OnMessage(matchMemeKeyword).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			info := ctx.State["meme_info"].(*MemeInfo)
			images := ctx.State["meme_images"].([]imageParam)
			texts := ctx.State["meme_texts"].([]string)
			var options map[string]interface{}
			if opt, ok := ctx.State["meme_options"]; ok {
				options = opt.(map[string]interface{})
			}
			doGenerate(ctx, info, images, texts, options)
		})
}

// ===================== 表情列表管理 =====================

func loadMemes() {
	infos, err := getMemeInfos()
	if err != nil {
		logrus.Errorf("[memes] 加载表情列表失败: %v", err)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	memeInfoMap = make(map[string]*MemeInfo, len(infos))
	keywordMap = make(map[string]*MemeInfo, len(infos)*3)
	shortcutMap = make(map[string]*shortcutEntry)
	shortcutRegexps = nil

	for i := range infos {
		info := &infos[i]
		memeInfoMap[info.Key] = info

		// 建立关键词索引
		for _, kw := range info.Keywords {
			keywordMap[strings.ToLower(kw)] = info
		}
		// key 自身也作为关键词
		keywordMap[strings.ToLower(info.Key)] = info

		// 建立快捷指令索引
		for _, sc := range info.Shortcuts {
			shortcutMap[sc.Pattern] = &shortcutEntry{info: info, shortcut: sc}
			re, err := regexp.Compile("^(?:" + sc.Pattern + ")$")
			if err != nil {
				logrus.Warnf("[memes] 编译快捷指令正则失败 %s: %v", sc.Pattern, err)
				continue
			}
			shortcutRegexps = append(shortcutRegexps, &shortcutMatcher{
				re:       re,
				info:     info,
				shortcut: sc,
			})
		}
	}

	logrus.Infof("[memes] 加载完成: %d 个表情, %d 个关键词, %d 个快捷指令",
		len(memeInfoMap), len(keywordMap), len(shortcutRegexps))
}

// findMemeInfo 通过关键词或 key 查找表情信息
func findMemeInfo(name string) *MemeInfo {
	lower := strings.ToLower(name)
	mu.RLock()
	defer mu.RUnlock()
	if info, ok := keywordMap[lower]; ok {
		return info
	}
	if info, ok := memeInfoMap[lower]; ok {
		return info
	}
	return nil
}

// ===================== 消息匹配 =====================

// imageParam 用于存储待使用的图片信息
type imageParam struct {
	name string // 名字（用户昵称等）
	url  string // 图片 URL
	data []byte // 图片数据（二选一）
}

// matchMemeKeyword 匹配消息中的表情关键词
func matchMemeKeyword(ctx *zero.Ctx) bool {
	msg := ctx.MessageString()
	if msg == "" {
		return false
	}

	// 提取纯文本部分（去掉 CQ 码）
	plainText := extractPlainText(ctx)
	if plainText == "" {
		return false
	}
	plainText = strings.TrimSpace(plainText)

	mu.RLock()
	defer mu.RUnlock()

	// 1. 先检查快捷指令（正则匹配完整消息）
	for _, sm := range shortcutRegexps {
		if sm.re.MatchString(plainText) {
			ctx.State["meme_info"] = sm.info
			ctx.State["meme_options"] = sm.shortcut.Options
			images, texts := extractParams(ctx)
			// 追加快捷指令中预设的文字
			texts = append(sm.shortcut.Texts, texts...)
			ctx.State["meme_images"] = images
			ctx.State["meme_texts"] = texts
			return true
		}
	}

	// 2. 通过关键词前缀匹配
	// 尝试找到最长匹配的关键词
	var bestInfo *MemeInfo
	var bestKeyword string
	for kw, info := range keywordMap {
		lower := strings.ToLower(plainText)
		if strings.HasPrefix(lower, kw) && len(kw) > len(bestKeyword) {
			// 确保关键词后面是空格、CQ码或结尾
			rest := plainText[len(kw):]
			if rest == "" || rest[0] == ' ' || rest[0] == '[' || rest[0] == '\n' {
				bestInfo = info
				bestKeyword = kw
			}
		}
	}

	if bestInfo == nil {
		return false
	}

	// 提取关键词后面的参数
	ctx.State["meme_info"] = bestInfo
	images, texts := extractParams(ctx)

	// 从关键词后面提取额外的文字参数
	rest := strings.TrimSpace(plainText[len(bestKeyword):])
	if rest != "" {
		// 过滤掉 CQ 码部分
		cqRe := regexp.MustCompile(`\[CQ:[^\]]+\]`)
		cleanRest := strings.TrimSpace(cqRe.ReplaceAllString(rest, ""))
		if cleanRest != "" {
			// 按空格分割文字参数
			parts := strings.Fields(cleanRest)
			for _, p := range parts {
				if p == "自己" {
					// "自己" -> 使用发送者头像
					uid := ctx.Event.UserID
					avatarURL := fmt.Sprintf("https://q4.qlogo.cn/g?b=qq&nk=%d&s=640", uid)
					name := ctx.CardOrNickName(uid)
					images = append(images, imageParam{name: name, url: avatarURL})
				} else if strings.HasPrefix(p, "#") {
					// 忽略 #name 标记，已在图片名中处理
				} else {
					texts = append(texts, p)
				}
			}
		}
	}

	ctx.State["meme_images"] = images
	ctx.State["meme_texts"] = texts
	return true
}

// extractPlainText 从消息中提取纯文本
func extractPlainText(ctx *zero.Ctx) string {
	var sb strings.Builder
	for _, seg := range ctx.Event.Message {
		if seg.Type == "text" {
			sb.WriteString(seg.Data["text"])
		}
	}
	return sb.String()
}

// extractParams 从消息中提取图片和文字参数
func extractParams(ctx *zero.Ctx) (images []imageParam, texts []string) {
	for _, seg := range ctx.Event.Message {
		switch seg.Type {
		case "image":
			url := seg.Data["url"]
			if url == "" {
				url = seg.Data["file"]
			}
			if url != "" {
				images = append(images, imageParam{name: "", url: url})
			}
		case "at":
			qq := seg.Data["qq"]
			if qq != "" && qq != "all" {
				uid, err := strconv.ParseInt(qq, 10, 64)
				if err == nil {
					avatarURL := fmt.Sprintf("https://q4.qlogo.cn/g?b=qq&nk=%d&s=640", uid)
					name := ctx.CardOrNickName(uid)
					images = append(images, imageParam{name: name, url: avatarURL})
				}
			}
		}
	}
	return
}

// ===================== 表情生成 =====================

func doGenerate(ctx *zero.Ctx, info *MemeInfo, images []imageParam, texts []string, options map[string]interface{}) {
	params := info.Params

	// 当所需图片数为 2 且已指定 1 张图片时，用发送者头像作为第一张
	if params.MinImages == 2 && len(images) == 1 {
		uid := ctx.Event.UserID
		avatarURL := fmt.Sprintf("https://q4.qlogo.cn/g?b=qq&nk=%d&s=640", uid)
		name := ctx.CardOrNickName(uid)
		images = append([]imageParam{{name: name, url: avatarURL}}, images...)
	}

	// 当所需图片数为 1 且没有图片时，使用发送者头像
	if params.MinImages >= 1 && len(images) == 0 {
		uid := ctx.Event.UserID
		avatarURL := fmt.Sprintf("https://q4.qlogo.cn/g?b=qq&nk=%d&s=640", uid)
		name := ctx.CardOrNickName(uid)
		images = append(images, imageParam{name: name, url: avatarURL})
	}

	// 如果还是不够图片，提示
	if len(images) < params.MinImages {
		num := strconv.Itoa(params.MinImages)
		if params.MaxImages > params.MinImages {
			num += " ~ " + strconv.Itoa(params.MaxImages)
		}
		ctx.SendChain(message.Text(fmt.Sprintf("图片数量不足，需要 %s 张图片", num)))
		return
	}

	// 截断多余图片
	if params.MaxImages > 0 && len(images) > params.MaxImages {
		images = images[:params.MaxImages]
	}

	// 当需要文字但没有输入时，使用默认文字
	if params.MinTexts > 0 && len(texts) == 0 && len(params.DefaultTexts) > 0 {
		texts = params.DefaultTexts
	}

	// 文字数量检查
	if len(texts) < params.MinTexts {
		num := strconv.Itoa(params.MinTexts)
		if params.MaxTexts > params.MinTexts {
			num += " ~ " + strconv.Itoa(params.MaxTexts)
		}
		ctx.SendChain(message.Text(fmt.Sprintf("文字数量不足，需要 %s 段文字", num)))
		return
	}
	if params.MaxTexts > 0 && len(texts) > params.MaxTexts {
		texts = texts[:params.MaxTexts]
	}

	// 上传图片并收集 MemeImage
	memeImages := make([]MemeImage, 0, len(images))
	for _, img := range images {
		var imageID string
		var err error

		if img.url != "" {
			// 先尝试通过 URL 直接上传给 meme-generator-rs
			imageID, err = uploadImageByURL(img.url)
			if err != nil {
				// 如果 URL 上传失败，尝试先下载再上传
				logrus.Debugf("[memes] URL 上传失败，尝试下载后上传: %v", err)
				data, dlErr := web.GetData(img.url)
				if dlErr != nil {
					ctx.SendChain(message.Text("图片下载失败: ", dlErr))
					return
				}
				imageID, err = uploadImage(data)
				if err != nil {
					ctx.SendChain(message.Text("图片上传失败: ", err))
					return
				}
			}
		} else if img.data != nil {
			imageID, err = uploadImage(img.data)
			if err != nil {
				ctx.SendChain(message.Text("图片上传失败: ", err))
				return
			}
		} else {
			continue
		}

		name := img.name
		if name == "" {
			name = "image"
		}
		memeImages = append(memeImages, MemeImage{Name: name, ID: imageID})
	}

	// 生成表情
	result, err := generateMeme(info.Key, memeImages, texts, options)
	if err != nil {
		if memeErr, ok := err.(*MemeGeneratorError); ok {
			ctx.SendChain(message.Text("表情生成失败: ", memeErr.Detail))
		} else {
			ctx.SendChain(message.Text("表情生成失败: ", err))
		}
		return
	}

	b64 := base64.StdEncoding.EncodeToString(result)
	ctx.SendChain(message.Image("base64://" + b64))
}
