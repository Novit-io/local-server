package main

import (
	"flag"
	"log"
	"path/filepath"

	"novit.nc/direktil/pkg/clustersconfig"
)

var (
	dataDir       = flag.String("data", "/var/lib/direktil", "Data dir")
	configFromDir = flag.String("config-from-dir", "", "Build configuration from this directory")
)

func readConfig() (config *clustersconfig.Config, err error) {
	configFile := filepath.Join(*dataDir, "current-config.yaml")

	if *configFromDir != "" {
		config, err = clustersconfig.FromDir(*configFromDir)
		if err != nil {
			log.Print("failed to load config: ", err)
			return nil, err
		}

		if err = config.SaveTo(configFile); err != nil {
			return nil, err
		}

		return
	}

	return clustersconfig.FromFile(configFile)
}
