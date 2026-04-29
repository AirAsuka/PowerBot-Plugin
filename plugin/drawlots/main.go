// Package drawlots 多功能抽签插件
package drawlots

import (
	"bytes"
	"errors"
	"image"
	"image/gif"
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/FloatTech/floatbox/file"
	"github.com/FloatTech/floatbox/web"
<<<<<<< HEAD
=======
	"github.com/FloatTech/gg/factory"
>>>>>>> cd4090c7bfc5dace579da4bcd08c28ac9c8138de
	ctrl "github.com/FloatTech/zbpctrl"
	control "github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/FloatTech/zbputils/img/text"
	"github.com/fumiama/jieba/util/helper"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

type info struct {
	lotsType string
	quantity int
}

var (
	lotsList = func() map[string]info {
		lotsList, err := getList()
		if err != nil {
			logrus.Infoln("[drawlots]加载失败:", err, "(如果从未使用过该插件, 这是正常现象)")
		} else {
			logrus.Infoln("[drawlots]加载", len(lotsList), "个抽签")
		}
		return lotsList
	}()
	en = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "多功能抽签",
		Help: "支持图包和gif抽签\n" +
			"-------------\n" +
			"- (刷新)抽签列表\n- 抽[签名]签\n- 抽群友签(随机图包)\n- 看[签名]签\n- 加[签名]签[图片]\n- 删[签名]签\n- 抽[签名]gif签\n- 看[签名]gif签\n- 加[签名]gif签[gif]\n- 删[签名]gif签",
		PrivateDataFolder: "drawlots",
	}).ApplySingle(ctxext.DefaultSingle)
	datapath = file.BOTPATH + "/" + en.DataFolder()
)

func init() {
	en.OnFullMatchGroup([]string{"抽签列表", "刷新抽签列表"}).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		var err error
		lotsList, err = getList()
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		messageText := &strings.Builder{}
		messageText.WriteString("    ◆ 抽签列表 ◆\n")
		messageText.WriteString("─────────────────\n\n")
		folderCount := 0
		gifCount := 0
		for name, fileInfo := range lotsList {
			typeName := "图包"
			marker := "◈"
			if fileInfo.lotsType == "gif" {
				typeName = "GIF"
				marker = "◇"
				gifCount++
			} else {
				folderCount++
			}
			messageText.WriteString("  " + marker + " " + name + "\n")
			messageText.WriteString("     " + typeName + " · " + strconv.Itoa(fileInfo.quantity) + " 签\n\n")
		}
		messageText.WriteString("─────────────────\n")
		messageText.WriteString("  图包 " + strconv.Itoa(folderCount) + " 种")
		messageText.WriteString(" | GIF " + strconv.Itoa(gifCount) + " 种")
		messageText.WriteString(" | 共 " + strconv.Itoa(len(lotsList)) + " 种")
		textPic, err := text.RenderToBase64(messageText.String(), text.BoldFontFile, 400, 30)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Image("base64://" + helper.BytesToString(textPic)))
	})

	en.OnRegex(`^抽(.+)签$`).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		lotsType := ctx.State["regex_matched"].([]string)[1]
		if lotsType == "群友" {
			if len(lotsList) == 0 {
				ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.Text("还没有任何签呢~")))
				return
			}
			totalQuantity := 0
			for _, fi := range lotsList {
				if fi.lotsType == "folder" {
					totalQuantity += fi.quantity
				}
			}
			if totalQuantity == 0 {
				ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.Text("没有图包签可以抽呢~")))
				return
			}
			randNum := rand.Intn(totalQuantity)
			for name, fi := range lotsList {
				if fi.lotsType != "folder" {
					continue
				}
				randNum -= fi.quantity
				if randNum < 0 {
					picPath, err := randFile(name, 3)
					if err != nil {
						ctx.SendChain(message.Text("ERROR: ", err))
						return
					}
					ctx.SendChain(message.Reply(ctx.Event.MessageID), message.Image("file:///"+picPath))
					return
				}
			}
			return
		}
		fileInfo, ok := lotsList[lotsType]
		if !ok {
			ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.Text("才...才没有", lotsType, "签啦~")))
			return
		}
		if fileInfo.lotsType != "folder" {
			ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.Text("这是GIF签，请使用\"抽", lotsType, "gif签\"哦~")))
			return
		}
		picPath, err := randFile(lotsType, 3)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
<<<<<<< HEAD
		ctx.SendChain(message.Reply(ctx.Event.MessageID), message.Image("file:///"+picPath))
	})

	en.OnRegex(`^抽(.+)gif签$`).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		lotsName := ctx.State["regex_matched"].([]string)[1]
		fileInfo, ok := lotsList[lotsName]
		if !ok {
			ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.Text("才...才没有", lotsName, "gif签啦~")))
=======
		// 生成图片
		data, err := factory.ToBytes(lotsImg)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
>>>>>>> cd4090c7bfc5dace579da4bcd08c28ac9c8138de
			return
		}
		if fileInfo.lotsType != "gif" {
			ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.Text("这是图包签，请使用\"抽", lotsName, "签\"哦~")))
			return
		}
		ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.Image("file:///"+datapath+"/"+lotsName+".gif")))
	})

	en.OnRegex(`^看(.+)签$`, zero.UserOrGrpAdmin).SetBlock(true).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		id := ctx.Event.MessageID
		lotsName := ctx.State["regex_matched"].([]string)[1]
		fileInfo, ok := lotsList[lotsName]
		if !ok {
			ctx.Send(message.ReplyWithMessage(id, message.Text("才...才没有", lotsName, "签这种东西啦")))
			return
		}
		if fileInfo.lotsType != "folder" {
			ctx.Send(message.ReplyWithMessage(id, message.Text("这是GIF签，请使用\"看", lotsName, "gif签\"查看哦~")))
			return
		}
		picPath, err := randFile(lotsName, 3)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.Send(message.ReplyWithMessage(id, message.Image("file:///"+picPath)))
	})

	en.OnRegex(`^看(.+)gif签$`, zero.UserOrGrpAdmin).SetBlock(true).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		id := ctx.Event.MessageID
		lotsName := ctx.State["regex_matched"].([]string)[1]
		fileInfo, ok := lotsList[lotsName]
		if !ok {
			ctx.Send(message.ReplyWithMessage(id, message.Text("才...才没有", lotsName, "gif签这种东西啦")))
			return
		}
		if fileInfo.lotsType != "gif" {
			ctx.Send(message.ReplyWithMessage(id, message.Text("这是图包签，请使用\"看", lotsName, "签\"查看哦~")))
			return
		}
		ctx.Send(message.ReplyWithMessage(id, message.Image("file:///"+datapath+"/"+lotsName+".gif")))
	})

	en.OnRegex(`^加(.+)签.*`, zero.MustProvidePicture).SetBlock(true).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		id := ctx.Event.MessageID
		lotsName := ctx.State["regex_matched"].([]string)[1]
		if lotsName == "" {
			ctx.Send(message.ReplyWithMessage(id, message.Text("请使用正确的指令形式哦~")))
			return
		}
		if existing, ok := lotsList[lotsName]; ok && existing.lotsType == "gif" {
			ctx.Send(message.ReplyWithMessage(id, message.Text("已存在同名GIF签\"", lotsName, "\"，请换个名字或先删除GIF签哦~")))
			return
		}
		picURL := ctx.State["image_url"].([]string)[0]
		picData, err := web.GetData(picURL)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		_, format, err := image.DecodeConfig(bytes.NewReader(picData))
		if err != nil {
			ctx.SendChain(message.Text("ERROR: 不支持的图片格式"))
			return
		}
		dirPath := datapath + "/" + lotsName
		err = os.MkdirAll(dirPath, 0755)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		existFiles, _ := os.ReadDir(dirPath)
		newCount := len(existFiles) + 1
		fileName := filepath.Join(dirPath, strconv.Itoa(newCount)+"."+format)
		err = os.WriteFile(fileName, picData, 0644)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		lotsList[lotsName] = info{
			lotsType: "folder",
			quantity: newCount,
		}
		ctx.Send(message.ReplyWithMessage(id, message.Text("成功添加图片签！当前共", strconv.Itoa(newCount), "签")))
	})

	en.OnRegex(`^加(.+)gif签.*`, zero.MustProvidePicture).SetBlock(true).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		id := ctx.Event.MessageID
		lotsName := ctx.State["regex_matched"].([]string)[1]
		if lotsName == "" {
			ctx.Send(message.ReplyWithMessage(id, message.Text("请使用正确的指令形式哦~")))
			return
		}
		if existing, ok := lotsList[lotsName]; ok && existing.lotsType == "folder" {
			ctx.Send(message.ReplyWithMessage(id, message.Text("已存在同名图包签\"", lotsName, "\"（", strconv.Itoa(existing.quantity), "签），请换个名字或先手动移除图包哦~")))
			return
		}
		picURL := ctx.State["image_url"].([]string)[0]
		gifdata, err := web.GetData(picURL)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		im, err := gif.DecodeAll(bytes.NewReader(gifdata))
		if err != nil {
			ctx.Send(message.ReplyWithMessage(id, message.Text("这不是有效的GIF图片哦~")))
			return
		}
		fileName := filepath.Join(datapath, lotsName+".gif")
		err = os.WriteFile(fileName, gifdata, 0644)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		lotsList[lotsName] = info{
			lotsType: "gif",
			quantity: len(im.Image),
		}
		ctx.Send(message.ReplyWithMessage(id, message.Text("成功添加GIF签！共", strconv.Itoa(len(im.Image)), "签")))
	})

	en.OnRegex(`^删(.+)签$`, zero.SuperUserPermission).SetBlock(true).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		id := ctx.Event.MessageID
		lotsName := ctx.State["regex_matched"].([]string)[1]
		fileInfo, ok := lotsList[lotsName]
		if !ok {
			ctx.Send(message.ReplyWithMessage(id, message.Text("才...才没有", lotsName, "签这种东西啦")))
			return
		}
		if fileInfo.lotsType != "folder" {
			ctx.Send(message.ReplyWithMessage(id, message.Text("这是GIF签，请使用\"删", lotsName, "gif签\"删除哦~")))
			return
		}
		ctx.Send(message.ReplyWithMessage(id, message.Text("图包签请手动移除文件夹 "+datapath+"/"+lotsName+" 哦~")))
	})

	en.OnRegex(`^删(.+)gif签$`, zero.SuperUserPermission).SetBlock(true).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		id := ctx.Event.MessageID
		lotsName := ctx.State["regex_matched"].([]string)[1]
		fileInfo, ok := lotsList[lotsName]
		if !ok {
			ctx.Send(message.ReplyWithMessage(id, message.Text("才...才没有", lotsName, "gif签这种东西啦")))
			return
		}
		if fileInfo.lotsType != "gif" {
			ctx.Send(message.ReplyWithMessage(id, message.Text("这是图包签，请使用\"删", lotsName, "签\"删除哦~")))
			return
		}
		err := os.Remove(filepath.Join(datapath, lotsName+".gif"))
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		delete(lotsList, lotsName)
		if dirInfo, statErr := os.Stat(datapath + "/" + lotsName); statErr == nil && dirInfo.IsDir() {
			files, _ := os.ReadDir(datapath + "/" + lotsName)
			if len(files) > 0 {
				lotsList[lotsName] = info{
					lotsType: "folder",
					quantity: len(files),
				}
				ctx.Send(message.ReplyWithMessage(id, message.Text("GIF签已删除，已恢复同名图包签（", strconv.Itoa(len(files)), "签）")))
				return
			}
		}
		ctx.Send(message.ReplyWithMessage(id, message.Text("成功！")))
	})
}

func getList() (list map[string]info, err error) {
	list = make(map[string]info, 100)
	files, err := os.ReadDir(datapath)
	if err != nil {
		return
	}
	if len(files) == 0 {
		err = errors.New("什么签也没有哦~")
		return
	}
	for _, lots := range files {
		if lots.IsDir() {
			files, _ := os.ReadDir(datapath + "/" + lots.Name())
			list[lots.Name()] = info{
				lotsType: "folder",
				quantity: len(files),
			}
			continue
		}
		before, after, ok := strings.Cut(lots.Name(), ".")
		if !ok || before == "" {
			continue
		}
		file, err := os.Open(datapath + "/" + lots.Name())
		if err != nil {
			return nil, err
		}
		im, err := gif.DecodeAll(file)
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		list[before] = info{
			lotsType: after,
			quantity: len(im.Image),
		}
	}
	return
}

func randFile(path string, indexMax int) (string, error) {
	picPath := datapath + path
	files, err := os.ReadDir(picPath)
	if err != nil {
		return "", err
	}
	if len(files) > 0 {
		drawFile := files[rand.Intn(len(files))]
		// 如果是文件夹就递归
		if drawFile.IsDir() {
			indexMax--
			if indexMax <= 0 {
				return "", errors.New("图包[" + path + "]存在太多非图片文件,请清理~")
			}
			return randFile(path, indexMax)
		}
		return picPath + "/" + drawFile.Name(), err
	}
	return "", errors.New("图包[" + path + "]不存在签内容！")
}
