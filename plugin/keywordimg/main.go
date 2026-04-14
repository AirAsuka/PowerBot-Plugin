// Package keywordimg 关键词图片插件
package keywordimg

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

// KeywordImage 关键词图片结构
type KeywordImage struct {
	Keyword string `json:"keyword"`
	Image   string `json:"image"` // 本地图片路径
}

// 插件数据
var (
	keywordData = make(map[string]string) // keyword -> localImagePath
	dataFile    string
	imagesDir    string
	RWMutex      sync.RWMutex
	loadOnce     sync.Once
)

func init() {
	engine := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "关键词图片",
		Help: "- 加关键词 xxx [图片url]\n" +
			"- 加关键词 xxx (回复图片)\n" +
			"- 删关键词 xxx\n" +
			"- 查看关键词列表\n" +
			"\n当对话中检测到已添加的关键词时，自动返回对应的图片",
		PrivateDataFolder: "keywordimg",
	})

	// 初始化数据文件路径
	dataFile = engine.DataFolder() + "keywords.json"
	imagesDir = engine.DataFolder() + "images"

	// 加载数据
	loadData()

	// 确保图片目录存在
	os.MkdirAll(imagesDir, 0755)

	logrus.Infoln("[keywordimg] 插件加载完成，关键词数:", len(keywordData))

	// 关键词检测 - 在群聊中检测消息是否包含关键词
	engine.OnMessage(filter, zero.OnlyGroup).SetBlock(false).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		keyword := ctx.State["matched_keyword"].(string)
		RWMutex.RLock()
		imagePath, ok := keywordData[keyword]
		RWMutex.RUnlock()
		if ok && imagePath != "" {
			// 发送关键词对应的图片
			ctx.SendChain(message.Image("file:///" + imagePath))
		}
	})

	// 加关键词命令 (需要管理员权限)
	engine.OnPrefix("加关键词", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		uid := ctx.Event.UserID
		args := ctx.State["args"].(string)

		keyword := ""
		var imageURL string

		// 尝试从消息段中提取图片
		for _, seg := range ctx.Event.Message {
			if seg.Type == "image" {
				imageURL = seg.Data["url"]
				break
			}
		}

		// 解析关键词: 支持 "关键词" 或 "关键词 [图片url]"
		// 也支持 "关键词[CQ:image,..." (关键词和图片紧贴)
		trimmedArgs := strings.TrimSpace(args)

		// 尝试查找 CQ:image 模式的位置
		cqIndex := strings.Index(trimmedArgs, "[CQ:image")
		if cqIndex > 0 {
			// 格式: 关键词[CQ:image,...]
			keyword = strings.TrimSpace(trimmedArgs[:cqIndex])
		} else {
			// 尝试空格分割
			parts := strings.SplitN(trimmedArgs, " ", 2)
			keyword = strings.TrimSpace(parts[0])
			if len(parts) >= 2 && imageURL == "" {
				// parts[1] 可能是图片URL
				imageURL = strings.TrimSpace(parts[1])
			}
		}

		if keyword == "" {
			ctx.SendChain(message.Text("格式错误，请使用: 加关键词 xxx [图片]"))
			return
		}

		if imageURL == "" {
			// 格式2: 需要用户回复一张图片
			ctx.SendChain(message.Text("请回复一张图片来设置关键词 ", keyword, " 的图片"))

			// 等待用户回复图片
			recv, cancel := zero.NewFutureEvent("message", 999, false, zero.CheckUser(uid), zero.CheckGroup(ctx.Event.GroupID)).Repeat()
			defer cancel()
			timer := time.NewTimer(120 * time.Second)
			defer timer.Stop()

			for {
				select {
				case <-timer.C:
					ctx.SendChain(message.Text("等待超时，添加关键词已取消"))
					return
				case c := <-recv:
					// 检查是否有图片
					for _, seg := range c.Event.Message {
						if seg.Type == "image" {
							imageURL = seg.Data["url"]
							goto done
						}
					}
					// 没有图片，继续等待
					ctx.SendChain(message.Text("没有检测到图片，请回复一张图片"))
				}
			}
		done:
		}

		if imageURL == "" {
			ctx.SendChain(message.Text("图片不能为空"))
			return
		}

		if imageURL == "" {
			ctx.SendChain(message.Text("图片不能为空"))
			return
		}

		// 下载图片到本地
		ctx.SendChain(message.Text("正在下载图片..."))
		localPath, err := downloadImage(imageURL, keyword)
		if err != nil {
			ctx.SendChain(message.Text("下载图片失败: ", err))
			return
		}

		// 如果已存在相同关键词的图片，先删除旧图片
		RWMutex.Lock()
		if oldPath, ok := keywordData[keyword]; ok {
			os.Remove(oldPath)
		}
		keywordData[keyword] = localPath
		saveData()
		RWMutex.Unlock()

		ctx.SendChain(message.Text("关键词 [", keyword, "] 添加成功"))
	})

	// 删关键词命令 (需要管理员权限)
	engine.OnPrefix("删关键词", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		args := strings.TrimSpace(ctx.State["args"].(string))
		if args == "" {
			ctx.SendChain(message.Text("请指定要删除的关键词"))
			return
		}

		RWMutex.Lock()
		defer RWMutex.Unlock()

		if imagePath, ok := keywordData[args]; ok {
			// 删除图片文件
			os.Remove(imagePath)
			delete(keywordData, args)
			saveData()
			ctx.SendChain(message.Text("关键词 [", args, "] 已删除"))
		} else {
			ctx.SendChain(message.Text("关键词 [", args, "] 不存在"))
		}
	})

	// 查看关键词列表命令
	engine.OnFullMatch("查看关键词列表", zero.OnlyGroup).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		RWMutex.RLock()
		defer RWMutex.RUnlock()

		if len(keywordData) == 0 {
			ctx.SendChain(message.Text("当前没有任何关键词"))
			return
		}

		var list strings.Builder
		list.WriteString("当前关键词列表:\n")
		for keyword := range keywordData {
			list.WriteString("- ")
			list.WriteString(keyword)
			list.WriteString("\n")
		}
		ctx.SendChain(message.Text(list.String()))
	})
}

// filter 消息过滤器，检测消息中是否包含关键词
func filter(ctx *zero.Ctx) bool {
	// 获取消息纯文本内容
	text := ctx.Event.Message.ExtractPlainText()
	if text == "" {
		return false
	}

	// 跳过以命令前缀开头的消息
	if strings.HasPrefix(text, "加关键词") || strings.HasPrefix(text, "删关键词") || strings.HasPrefix(text, "查看关键词") {
		return false
	}

	RWMutex.RLock()
	defer RWMutex.RUnlock()

	// 调试
	println("[keywordimg] 检查消息:", text, "数据条数:", len(keywordData))

	// 检查消息是否包含任何关键词
	for keyword, path := range keywordData {
		println("[keywordimg] 遍历关键词:", keyword, "路径:", path)
		if strings.Contains(text, keyword) {
			ctx.State["matched_keyword"] = keyword
			println("[keywordimg] 匹配成功:", keyword)
			return true
		}
	}
	return false
}

// downloadImage 下载图片到本地目录
func downloadImage(url, keyword string) (string, error) {
	// 获取图片扩展名
	ext := getImageExt(url)
	if ext == "" {
		ext = ".jpg"
	}

	// 生成文件名: keyword_md5hash.ext
	hash := md5hash(url)
	filename := fmt.Sprintf("%s_%s%s", keyword, hash, ext)
	localPath := filepath.Join(imagesDir, filename)

	// 如果文件已存在，直接返回
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	// 下载图片
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http status: %d", resp.StatusCode)
	}

	// 创建临时文件
	tmpFile := localPath + ".tmp"
	out, err := os.Create(tmpFile)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	// 关闭文件后再重命名
	out.Close()

	// 原子性重命名
	if err := os.Rename(tmpFile, localPath); err != nil {
		return "", err
	}

	// 验证文件是否存在
	if info, err := os.Stat(localPath); err != nil || info.Size() == 0 {
		os.Remove(localPath)
		return "", fmt.Errorf("invalid file")
	}

	return localPath, nil
}

// getImageExt 从URL或Content-Type获取图片扩展名
func getImageExt(url string) string {
	// 尝试从URL路径中获取扩展名
	url = strings.ToLower(url)
	if idx := strings.LastIndex(url, "."); idx != -1 {
		ext := url[idx:]
		if len(ext) <= 5 {
			// 常见的图片扩展名
			switch ext {
			case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
				return ext
			}
		}
	}
	return ""
}

// loadData 从文件加载关键词数据
func loadData() {
	loadOnce.Do(func() {
		// 确保数据目录存在
		dir := filepath.Dir(dataFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return
		}

		data, err := os.ReadFile(dataFile)
		if err != nil {
			return
		}

		var keywords []KeywordImage
		if err := json.Unmarshal(data, &keywords); err != nil {
			return
		}

		for _, kw := range keywords {
			// 验证图片文件是否存在
			if _, err := os.Stat(kw.Image); err == nil {
				keywordData[kw.Keyword] = kw.Image
			}
		}
	})
}

// saveData 保存关键词数据到文件
func saveData() {
	keywords := make([]KeywordImage, 0, len(keywordData))
	RWMutex.RLock()
	for keyword, imagePath := range keywordData {
		keywords = append(keywords, KeywordImage{
			Keyword: keyword,
			Image:   imagePath,
		})
	}
	RWMutex.RUnlock()

	data, err := json.Marshal(keywords)
	if err != nil {
		return
	}

	// 确保目录存在
	dir := filepath.Dir(dataFile)
	os.MkdirAll(dir, 0755)

	// 写入文件
	tmpFile := dataFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return
	}

	// 原子性重命名
	os.Rename(tmpFile, dataFile)
}

// md5hash 计算字符串的MD5哈希
func md5hash(s string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s)))
}
