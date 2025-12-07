package main

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

// 基于游标的分页
type Page struct {
	NextID        string `json:"next_id"`
	NextTimeAtUTC int64  `json:"next_time_at_utc"`
	PageSize      int64  `json:"page_size"`
}

type Token string

func (p Page) InValid() bool {
	return p.NextID == "" || p.NextTimeAtUTC == 0 || p.NextTimeAtUTC > time.Now().Unix() || p.PageSize <= 0
}

// Encode 返回分布token
func (p Page) Encode() Token {
	b, err := json.Marshal(p)
	if err != nil {
		return Token("")
	}
	return Token(base64.StdEncoding.EncodeToString(b))
}

// Decode 解析分布信息
func (t Token) DeCode() Page {
	var result Page
	if len(t) == 0 {
		return result
	}

	bytes, err := base64.StdEncoding.DecodeString(string(t))
	if err != nil {
		return result
	}

	err = json.Unmarshal(bytes, &result)
	if err != nil {
		return result
	}
	return result
}
