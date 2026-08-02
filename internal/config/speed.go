package config

import (
	"fmt"
	"strconv"
	"strings"
)

// parseSpeedBytes 解析 aria2 风格速度值（支持 K/M/G 后缀，基数 1024）。
func parseSpeedBytes(value string) (int64, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, fmt.Errorf("empty speed value")
	}
	multiplier := int64(1)
	if len(text) >= 2 {
		switch strings.ToUpper(text[len(text)-1:]) {
		case "K":
			multiplier = 1024
			text = text[:len(text)-1]
		case "M":
			multiplier = 1024 * 1024
			text = text[:len(text)-1]
		case "G":
			multiplier = 1024 * 1024 * 1024
			text = text[:len(text)-1]
		}
	}
	base, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return 0, err
	}
	return base * multiplier, nil
}
