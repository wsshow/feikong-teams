package version

import (
	"fmt"
)

var (
	version   string = "0.0.1"
	buildTime string = "2025-01-01 00:00:00"
)

type Info struct {
	Version   string `json:"version,omitempty"`
	BuildTime string `json:"buildDate,omitempty"`
}

func (info Info) String() string {
	return fmt.Sprintf("Version:%s BuildDate:%s", info.Version, info.BuildTime)
}

func Get() Info {
	return Info{
		Version:   version,
		BuildTime: buildTime,
	}
}
