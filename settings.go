package main

import (
	"github.com/pelletier/go-toml"
)

type Settings struct {
	Folder    string
	DbName    string
	Interval  uint64
	Extension string
}

func loadSettings(filename string) *Settings {
	s := &Settings{}
	config, err := toml.LoadFile(filename)
	handleErr(err)

	if config.Has("folder") {
		s.Folder = config.Get("folder").(string)
	}

	if config.Has("db_name") {
		s.DbName = config.Get("db_name").(string)
	}

	if config.Has("interval_minutes") {
		s.Interval = uint64(config.Get("interval_minutes").(int64))
	}

	if config.Has("video_extension") {
		s.Extension = config.Get("video_extension").(string)
	}

	return s
}
