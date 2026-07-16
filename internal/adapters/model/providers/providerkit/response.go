package providerkit

import (
	"encoding/json"
	"fmt"
	"io"
)

const MaxJSONResponseBytes int64 = 8 << 20

// DecodeJSONResponse 在固定资源边界内解码外部服务响应。
func DecodeJSONResponse(reader io.Reader, target any) error {
	data, err := io.ReadAll(io.LimitReader(reader, MaxJSONResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read JSON response: %w", err)
	}
	if int64(len(data)) > MaxJSONResponseBytes {
		return fmt.Errorf("JSON response exceeds %d bytes", MaxJSONResponseBytes)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode JSON response: %w", err)
	}
	return nil
}
