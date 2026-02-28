// Package shindan 基于 https://shindanmaker.com 的测定小功能
package shindan

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	xpath "github.com/antchfx/htmlquery"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
	"github.com/wdvxdr1123/ZeroBot/utils/helper"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/FloatTech/zbputils/img/text"
)

func init() {
	engine := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "shindan测定",
		Help: "- 今天是什么少女[@xxx]\n" +
			"- 异世界转生[@xxx]\n" +
			"- 卖萌[@xxx]\n" +
			"- 今日老婆[@xxx]\n" +
			"- 黄油角色[@xxx]",
	})
	engine.OnPrefix("异世界转生", number(587874)).SetBlock(true).Limit(ctxext.LimitByUser).Handle(handlepic)
	engine.OnPrefix("今天是什么少女", number(162207)).SetBlock(true).Limit(ctxext.LimitByUser).Handle(handlepic)
	engine.OnPrefix("卖萌", number(360578)).SetBlock(true).Limit(ctxext.LimitByUser).Handle(handletxt)
	engine.OnPrefix("今日老婆", number(1075116)).SetBlock(true).Limit(ctxext.LimitByUser).Handle(handlecq)
	engine.OnPrefix("黄油角色", number(1115465)).SetBlock(true).Limit(ctxext.LimitByUser).Handle(handlepic)
}

func handletxt(ctx *zero.Ctx) {
	// 获取名字
	name := ctx.NickName()
	// 调用接口
	txt, err := getShindanResult(ctx.State["id"].(int64), name)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: ", err))
		return
	}
	ctx.SendChain(message.Text(txt))
}

func handlecq(ctx *zero.Ctx) {
	// 获取名字
	name := ctx.NickName()
	// 调用接口
	txt, err := getShindanResult(ctx.State["id"].(int64), name)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: ", err))
		return
	}
	ctx.Send(txt)
}

func handlepic(ctx *zero.Ctx) {
	// 获取名字
	name := ctx.NickName()
	// 调用接口
	txt, err := getShindanResult(ctx.State["id"].(int64), name)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: ", err))
		return
	}
	data, err := text.RenderToBase64(txt, text.FontFile, 400, 20)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: ", err))
		return
	}
	if id := ctx.SendChain(message.Image("base64://" + helper.BytesToString(data))); id.ID() == 0 {
		ctx.SendChain(message.Text("ERROR: 可能被风控了"))
	}
}

// 传入 shindanmaker id
func number(id int64) func(ctx *zero.Ctx) bool {
	return func(ctx *zero.Ctx) bool {
		ctx.State["id"] = id
		return true
	}
}

func getShindanResult(id int64, name string) (string, error) {
	url := fmt.Sprintf("https://shindanmaker.com/%d", id)
	seed := dailySeed()
	name += seed

	token, cookie, err := getTokenAndCookie(url)
	if err != nil {
		return "", err
	}

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("_token", token)
	_ = writer.WriteField("user_input_value_1", name)
	_ = writer.WriteField("randname", "名無しのR")
	_ = writer.WriteField("type", "name")
	_ = writer.Close()

	req, err := http.NewRequest(http.MethodPost, url, payload)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := xpath.Parse(resp.Body)
	if err != nil {
		return "", err
	}

	list := xpath.Find(doc, `//*[@id="shindanResult"]`)
	if len(list) == 0 {
		return "", errors.New("无法查找到结果, 请稍后再试")
	}

	output := make([]string, 0, 8)
	for child := list[0].FirstChild; child != nil; child = child.NextSibling {
		txt := xpath.InnerText(child)
		switch {
		case txt != "":
			output = append(output, txt)
		case child.Data == "img":
			img := child.Attr[1].Val
			if strings.Contains(img, "http") {
				output = append(output, "[CQ:image,file="+img[strings.Index(img, ",")+1:]+"]")
			} else {
				output = append(output, "[CQ:image,file=base64://"+img[strings.Index(img, ",")+1:]+"]")
			}
		default:
			output = append(output, "\n")
		}
	}
	return strings.ReplaceAll(strings.Join(output, ""), seed, ""), nil
}

func getTokenAndCookie(url string) (token, cookie string, err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	temp := resp.Header.Values("Set-Cookie")
	if len(temp) == 0 {
		return "", "", errors.New("刷新 cookie 时发生错误")
	}
	cookie = temp[len(temp)-1]
	if !strings.Contains(cookie, "_session") {
		return "", "", errors.New("刷新 cookie 时发生错误")
	}

	doc, err := xpath.Parse(resp.Body)
	if err != nil {
		return "", "", err
	}
	list := xpath.Find(doc, `//*[@id="shindanForm"]/input`)
	if len(list) == 0 {
		return "", "", errors.New("刷新 token 时发生错误")
	}
	token = list[0].Attr[2].Val
	return token, cookie, nil
}

func dailySeed() string {
	now := time.Now()
	return fmt.Sprintf("%d%d%d", now.Year(), now.Month(), now.Day())
}
