package main

import (
	"flag"
	"path/filepath"

	"novit.nc/direktil/pkg/localconfig"
)

var (
	dataDir       = flag.String("data", "/var/lib/direktil", "Data dir")
	configFromDir = flag.String("config-from-dir", "", "Build configuration from this directory")
)

func readConfig() (config *localconfig.Config, err error) {
	configFile := filepath.Join(*dataDir, "config.yaml")

	return localconfig.FromFile(configFile)
}
