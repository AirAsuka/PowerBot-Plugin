package memes

type MemeInfo struct {
	Key          string     `json:"key"`
	Keywords     []string   `json:"keywords"`
	Patterns     []string   `json:"patterns"`
	Tags         []string   `json:"tags"`
	Params       MemeParams `json:"params"`
	Shortcuts    []Shortcut `json:"shortcuts,omitempty"`
	DateCreated  string     `json:"date_created"`
	DateModified string     `json:"date_modified"`
}

type Shortcut struct {
	Pattern   string         `json:"pattern"`
	Humanized *string        `json:"humanized,omitempty"`
	Names     []string       `json:"names,omitempty"`
	Texts     []string       `json:"texts,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
}

type MemeParams struct {
	MinImages    int       `json:"min_images"`
	MaxImages    int       `json:"max_images"`
	MinTexts     int       `json:"min_texts"`
	MaxTexts     int       `json:"max_texts"`
	DefaultTexts []string  `json:"default_texts"`
	ArgsType     *ArgsType `json:"args_type,omitempty"`
}

type ArgsType struct {
	ArgsModel     *ArgsModel     `json:"args_model,omitempty"`
	ParserOptions []ParserOption `json:"parser_options,omitempty"`
}

type ArgsModel struct {
	Properties map[string]PropertyInfo `json:"properties,omitempty"`
}

type PropertyInfo struct {
	Type        string      `json:"type,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Description string      `json:"description,omitempty"`
	Minimum     *float64    `json:"minimum,omitempty"`
	Maximum     *float64    `json:"maximum,omitempty"`
}

type ParserOption struct {
	Names    []string    `json:"names,omitempty"`
	Dest     string      `json:"dest,omitempty"`
	HelpText string      `json:"help_text,omitempty"`
	Action   *ActionInfo `json:"action,omitempty"`
}

type ActionInfo struct {
	Type  int         `json:"type"`
	Value interface{} `json:"value,omitempty"`
}

type ImageUploadRequest struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
	Data string `json:"data,omitempty"`
	Path string `json:"path,omitempty"`
}

type ImageResponse struct {
	ImageID string `json:"image_id"`
}

type MemeImage struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type MemeGenerateRequest struct {
	Images  []MemeImage            `json:"images"`
	Texts   []string               `json:"texts"`
	Options map[string]interface{} `json:"options"`
}

type RenderListRequest struct {
	SortBy string `json:"sort_by,omitempty"`
}

type MemeAPIError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"data"`
}

func (e *MemeAPIError) Error() string {
	return e.Message
}

func (e *MemeAPIError) UserMessage() string {
	switch e.Code {
	case 410:
		return "请求错误，请检查输入格式"
	case 420:
		return "IO错误，图片处理失败"
	case 510:
		return "图片解码错误，请确保图片格式正确"
	case 520:
		return "图片编码错误"
	case 530:
		if m, ok := e.Details.(map[string]interface{}); ok {
			if path, ok := m["path"].(string); ok {
				return "缺少图片资源: " + path
			}
		}
		return "缺少图片资源"
	case 540:
		return "表情选项解析错误"
	case 550:
		if m, ok := e.Details.(map[string]interface{}); ok {
			min, _ := m["min"].(float64)
			max, _ := m["max"].(float64)
			if min == max {
				return "图片数量不符，需要 " + itoa(int(min)) + " 张图片"
			}
			return "图片数量不符，需要 " + itoa(int(min)) + " ~ " + itoa(int(max)) + " 张图片"
		}
		return "图片数量不符"
	case 551:
		if m, ok := e.Details.(map[string]interface{}); ok {
			min, _ := m["min"].(float64)
			max, _ := m["max"].(float64)
			if min == max {
				return "文字数量不符，需要 " + itoa(int(min)) + " 段文字"
			}
			return "文字数量不符，需要 " + itoa(int(min)) + " ~ " + itoa(int(max)) + " 段文字"
		}
		return "文字数量不符"
	case 560:
		if m, ok := e.Details.(map[string]interface{}); ok {
			if text, ok := m["text"].(string); ok {
				if len(text) > 10 {
					return "文字过长: " + text[:10] + "..."
				}
				return "文字过长: " + text
			}
		}
		return "文字过长"
	case 570:
		if m, ok := e.Details.(map[string]interface{}); ok {
			if feedback, ok := m["feedback"].(string); ok {
				return feedback
			}
		}
		return "表情生成失败"
	default:
		return e.Message
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
