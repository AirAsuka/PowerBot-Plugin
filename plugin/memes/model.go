package memes

type MemeInfo struct {
	Key          string     `json:"key"`
	Keywords     []string   `json:"keywords"`
	Patterns     []string   `json:"patterns"`
	Tags         []string   `json:"tags"`
	Params       MemeParams `json:"params"`
	DateCreated  string     `json:"date_created"`
	DateModified string     `json:"date_modified"`
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
