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
	if *configFromDir != "" {
		config, err = clustersconfig.FromDir(*dataDir)
		if err != nil {
			log.Print("failed to load config: ", err)
			return nil, err
		}

		if err = config.SaveTo(filepath.Join(*dataDir, "global-config.yaml")); err != nil {
			return nil, err
		}

		return
	}

	return clustersconfig.FromFile(filepath.Join(*dataDir, "current-config.yaml"))
}
