package timer

import (
	"strconv"
	"strings"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func (t *Timer) sendmsg(grp int64, ctx *zero.Ctx) {
	ctx.Event = new(zero.Event)
	ctx.Event.GroupID = grp
	msg := make(message.Message, 0, 4)
	if t.AtQQ == "" {
		msg = append(msg, atall)
	} else {
		for _, qq := range strings.Split(t.AtQQ, ",") {
			uid, err := strconv.ParseInt(qq, 10, 64)
			if err == nil && uid > 0 {
				msg = append(msg, message.At(uid))
			}
		}
	}
	msg = append(msg, message.Text(t.Alert))
	if t.URL == "" {
		ctx.SendChain(msg...)
	} else {
		msg = append(msg, message.Image(t.URL).Add("cache", "0"))
		ctx.SendChain(msg...)
	}
}
