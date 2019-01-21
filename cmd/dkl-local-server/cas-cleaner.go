package main

import (
	"flag"
	"log"
	"sort"
	"time"
)

var (
	cacheCleanDelay = flag.Duration("cache-clean-delay", 10*time.Minute, "Time between cache cleanups")
)

func casCleaner() {
	for {
		err := cleanCAS()
		if err != nil {
			log.Print("warn: couldn't clean cache: ", err)
		}

		time.Sleep(*cacheCleanDelay)
	}
}

func cleanCAS() error {
	cfg, err := readConfig()
	if err != nil {
		return err
	}

	activeTags := make([]string, len(cfg.Hosts))

	for i, host := range cfg.Hosts {
		// FIXME ugly hack, same as in dir2config
		cfg, err := readConfig()
		if err != nil {
			return err
		}

		ctx, err := newRenderContext(host, cfg)
		if err != nil {
			return err
		}

		tag, err := ctx.Tag()
		if err != nil {
			return err
		}

		activeTags[i] = tag
	}

	tags, err := casStore.Tags()
	if err != nil {
		return err
	}

	sort.Strings(activeTags)

	for _, tag := range tags {
		idx := sort.SearchStrings(activeTags, tag)

		if idx < len(activeTags) && activeTags[idx] == tag {
			continue
		}

		// tag is not present in active tags
		log.Print("cache cleaner: removing tag ", tag)
		if err := casStore.Remove(tag); err != nil {
			log.Printf("cache cleaner: failed to remove tag %s: %v", tag, err)
		}
	}

	return nil
}
