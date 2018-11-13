package main

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

func rmTempFile(f *os.File) {
	f.Close()
	if err := os.Remove(f.Name()); err != nil {
		log.Print("failed to remove ", f.Name(), ": ", err)
	}
}

func buildBootTar(out io.Writer, ctx *renderContext) (err error) {
	grubCfg, err := ioutil.TempFile(os.TempDir(), "grub.cfg-")
	if err != nil {
		return
	}
	defer rmTempFile(grubCfg)

	_, err = grubCfg.Write(asset("grub.cfg"))
	if err != nil {
		return
	}
	grubCfg.Close()

	arch := tar.NewWriter(out)
	defer arch.Close()

	archAdd := func(path string, ba []byte) (err error) {
		err = arch.WriteHeader(&tar.Header{Name: path, Size: int64(len(ba))})
		if err != nil {
			return
		}
		_, err = arch.Write(ba)
		return
	}

	// config
	cfgBytes, cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	archAdd("config.yaml", cfgBytes)

	// add "current" elements
	type distCopy struct {
		Src []string
		Dst string
	}

	// kernel and initrd
	copies := []distCopy{
		{Src: []string{"kernels", ctx.Group.Kernel}, Dst: "current/vmlinuz"},
		{Src: []string{"initrd", ctx.Group.Initrd}, Dst: "current/initrd"},
	}

	// layers
	for _, layer := range cfg.Layers {
		layerVersion := ctx.Group.Versions[layer]
		if layerVersion == "" {
			return fmt.Errorf("layer %q not mapped to a version", layer)
		}

		copies = append(copies,
			distCopy{
				Src: []string{"layers", layer, layerVersion},
				Dst: filepath.Join("current", "layers", layer+".fs"),
			})
	}

	for _, copy := range copies {
		outPath, err := ctx.distFetch(copy.Src...)
		if err != nil {
			return err
		}

		f, err := os.Open(outPath)
		if err != nil {
			return err
		}

		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			return err
		}

		if err = arch.WriteHeader(&tar.Header{
			Name: copy.Dst,
			Size: stat.Size(),
		}); err != nil {
			return err
		}

		_, err = io.Copy(arch, f)
		if err != nil {
			return err
		}
	}

	return nil
}
