package config

import (
	"os"
)

func TZ() string {
	tz := os.Getenv("TZ")

	if tz == "" {
		return "Asia/Shanghai"
	}
	return tz
}
