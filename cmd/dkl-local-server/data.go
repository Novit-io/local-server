package main

import (
	"flag"
	"path/filepath"

	"novit.nc/direktil/pkg/localconfig"
)

var (
	dataDir = flag.String("data", "/var/lib/direktil", "Data dir")
)

func configFilePath() string {
	return filepath.Join(*dataDir, "config.yaml")
}

func readConfig() (config *localconfig.Config, err error) {
	return localconfig.FromFile(configFilePath())
}
