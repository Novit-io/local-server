package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	cpio "github.com/cavaliercoder/go-cpio"
	yaml "gopkg.in/yaml.v2"
)

func renderConfig(w http.ResponseWriter, r *http.Request, ctx *renderContext) error {
	log.Printf("sending config for %q", ctx.Host.Name)

	_, cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	ba, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/yaml")
	http.ServeContent(w, r, "config.yaml", time.Unix(0, 0), bytes.NewReader(ba))

	return nil
}

func buildInitrd(out io.Writer, ctx *renderContext) error {
	_, cfg, err := ctx.Config()

	if err != nil {
		return err
	}

	// send initrd basis
	initrdPath, err := ctx.distFetch("initrd", ctx.Host.Initrd)
	if err != nil {
		return err
	}

	err = writeFile(out, initrdPath)
	if err != nil {
		return err
	}

	// and our extra archive
	archive := cpio.NewWriter(out)

	// - required dirs
	for _, dir := range []string{
		"boot",
		"boot/current",
		"boot/current/layers",
	} {
		archive.WriteHeader(&cpio.Header{
			Name: dir,
			Mode: 0600 | cpio.ModeDir,
		})
	}

	// - the layers
	for _, layer := range cfg.Layers {
		layerVersion := ctx.Host.Versions[layer]
		if layerVersion == "" {
			return fmt.Errorf("layer %q not mapped to a version", layer)
		}

		path, err := ctx.distFetch("layers", layer, layerVersion)
		if err != nil {
			return err
		}

		stat, err := os.Stat(path)
		if err != nil {
			return err
		}

		archive.WriteHeader(&cpio.Header{
			Name: "boot/current/layers/" + layer + ".fs",
			Mode: 0600,
			Size: stat.Size(),
		})

		if err = writeFile(archive, path); err != nil {
			return err
		}
	}

	// - the configuration
	ba, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	archive.WriteHeader(&cpio.Header{
		Name: "boot/config.yaml",
		Mode: 0600,
		Size: int64(len(ba)),
	})

	archive.Write(ba)

	// finalize the archive
	archive.Flush()
	archive.Close()
	return nil
}
