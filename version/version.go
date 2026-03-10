// Package version 提供应用版本信息
package version

import (
	"fmt"
)

var (
	version   string = "0.0.1"
	buildTime string = "2025-01-01 00:00:00"
)

// Info 版本信息
type Info struct {
	Version   string `json:"version,omitempty"`
	BuildTime string `json:"buildDate,omitempty"`
}

// String 返回格式化的版本字符串
func (info Info) String() string {
	return fmt.Sprintf("%s (%s)", info.Version, info.BuildTime)
}

// Get 返回当前版本信息
func Get() Info {
	return Info{
		Version:   version,
		BuildTime: buildTime,
	}
}
