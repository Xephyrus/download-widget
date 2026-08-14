package downloader

import (
	"encoding/json"
	"fmt"
)

func decodeJSON(body []byte) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("解析 Pixiv 响应失败: %w", err)
	}
	return out, nil
}
