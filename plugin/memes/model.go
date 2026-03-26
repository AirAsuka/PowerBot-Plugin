// Package memes 表情包制作 - 数据模型
package memes

// MemeInfo 表情信息
type MemeInfo struct {
	Key        string   `json:"key"`
	Keywords   []string `json:"keywords"`
	Patterns   []string `json:"patterns"`
	Tags       []string `json:"tags"`
	Params     MemeParams `json:"params"`
	DateCreated  string `json:"date_created"`
	DateModified string `json:"date_modified"`
}

// MemeParams 表情参数
type MemeParams struct {
	MinImages    int      `json:"min_images"`
	MaxImages    int      `json:"max_images"`
	MinTexts     int      `json:"min_texts"`
	MaxTexts     int      `json:"max_texts"`
	DefaultTexts []string `json:"default_texts"`
	ArgsType     *ArgsType `json:"args_type,omitempty"`
}

// ArgsType 参数类型
type ArgsType struct {
	ArgsModel     *ArgsModel      `json:"args_model,omitempty"`
	ParserOptions []ParserOption  `json:"parser_options,omitempty"`
}

// ArgsModel 参数模型
type ArgsModel struct {
	Properties map[string]PropertyInfo `json:"properties,omitempty"`
}

// PropertyInfo 属性信息
type PropertyInfo struct {
	Type        string      `json:"type,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Description string      `json:"description,omitempty"`
	Minimum     *float64    `json:"minimum,omitempty"`
	Maximum     *float64    `json:"maximum,omitempty"`
}

// ParserOption 解析选项
type ParserOption struct {
	Names    []string      `json:"names,omitempty"`
	Dest     string        `json:"dest,omitempty"`
	HelpText string        `json:"help_text,omitempty"`
	Action   *ActionInfo   `json:"action,omitempty"`
}

// ActionInfo 动作信息
type ActionInfo struct {
	Type  int         `json:"type"`
	Value interface{} `json:"value,omitempty"`
}

// ImageUploadRequest 图片上传请求
type ImageUploadRequest struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
	Data string `json:"data,omitempty"`
	Path string `json:"path,omitempty"`
}

// ImageResponse 图片响应
type ImageResponse struct {
	ImageID string `json:"image_id"`
}

// MemeImage 表情图片参数
type MemeImage struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// MemeGenerateRequest 表情生成请求
type MemeGenerateRequest struct {
	Images  []MemeImage            `json:"images"`
	Texts   []string               `json:"texts"`
	Options map[string]interface{} `json:"options"`
}

// RenderListRequest 渲染列表请求
type RenderListRequest struct {
	SortBy string `json:"sort_by,omitempty"`
}
