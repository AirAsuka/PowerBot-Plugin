package fortune

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// cachedImage 缓存生成的图片，发送时使用内容而非本地路径。
func cachedImage(path string, generate func(io.Writer) error) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var buf bytes.Buffer
	if err := generate(&buf); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
