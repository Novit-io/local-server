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

func renderStaticPods(w http.ResponseWriter, r *http.Request, ctx *renderContext) error {
	log.Printf("sending static-pods for %q", ctx.Host.Name)

	ba, err := ctx.StaticPods()
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/yaml") // XXX can also be JSON
	http.ServeContent(w, r, "static-pods", time.Unix(0, 0), bytes.NewReader(ba))

	return nil
}

// TODO move somewhere logical
func renderCtx(w http.ResponseWriter, r *http.Request, ctx *renderContext, what string,
	create func(out io.Writer, ctx *renderContext) error) error {
	log.Printf("sending %s for %q", what, ctx.Host.Name)

	tag, err := ctx.Tag()
	if err != nil {
		return err
	}

	// get it or create it
	content, meta, err := casStore.GetOrCreate(tag, what, func(out io.Writer) error {
		log.Printf("building %s for %q", what, ctx.Host.Name)
		return create(out, ctx)
	})

	if err != nil {
		return err
	}

	// serve it
	http.ServeContent(w, r, what, meta.ModTime(), content)
	return nil
}

func buildInitrd(out io.Writer, ctx *renderContext) error {
	_, cfg, err := ctx.Config()

	if err != nil {
		return err
	}

	// send initrd basis
	initrdPath, err := ctx.distFetch("initrd", ctx.Group.Initrd)
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
		layerVersion := ctx.Group.Versions[layer]
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
