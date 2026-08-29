package aichat

import "testing"

func TestSanitizeAIReply(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			// 复现线上问题：AI 回复包含指令参数方括号（钱包转账[金额][@xxx]），
			// 旧 chat.Sanitize 会用 LastIndex 找到最后一个 "]" 并把之前的内容全部截掉，
			// 最终只发出 "&#93;」消息喵" 这种半截消息。
			name: "brackets in body must be preserved",
			in:   "可以试试发送「钱包转账[金额][@群友]」消息喵",
			want: "可以试试发送「钱包转账[金额][@群友]」消息喵",
		},
		{
			name: "fullwidth speaking prefix stripped",
			in:   "【嘿嘿】可以试试发送「钱包转账[金额]」消息喵",
			want: "可以试试发送「钱包转账[金额]」消息喵",
		},
		{
			name: "halfwidth speaking prefix stripped",
			in:   "[嘿嘿] 试试「钱包转账」消息喵",
			want: "试试「钱包转账」消息喵",
		},
		{
			name: "only first line kept",
			in:   "第一行\n第二行会被丢弃",
			want: "第一行",
		},
		{
			name: "leading fullwidth quote is not a prefix",
			in:   "「钱包转账」消息喵",
			want: "「钱包转账」消息喵",
		},
		{
			name: "empty stays empty",
			in:   "",
			want: "",
		},
		{
			name: "prefix only yields empty",
			in:   "【嘿嘿】",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeAIReply(c.in); got != c.want {
				t.Errorf("sanitizeAIReply(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
