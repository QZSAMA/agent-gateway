package store

import (
	"encoding/json"

	"github.com/agent-gateway/gateway/internal/model"
)

func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func unmarshalContentBlocks(s string) []model.ContentBlock {
	var blocks []model.ContentBlock
	json.Unmarshal([]byte(s), &blocks)
	return blocks
}
