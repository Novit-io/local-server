package main

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
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

	_, err = grubCfg.WriteString(`
search --no-floppy --set=root --part-label boot

insmod all_video
set timeout=3

set bootdev=PARTNAME=boot

menuentry "Direktil" {
    linux  /current/vmlinuz direktil.boot=$bootdev
    initrd /current/initrd
}
`)
	if err != nil {
		return
	}
	grubCfg.Close()

	// FIXME including in grub memdisk for now...
	kernelPath, err := ctx.distFetch("kernels", ctx.Group.Kernel)
	initrdPath, err := ctx.distFetch("initrd", ctx.Group.Initrd)

	grubMk := func(format string) ([]byte, error) {
		grubOut, err := ioutil.TempFile(os.TempDir(), "grub.img-")
		if err != nil {
			return nil, err
		}
		defer rmTempFile(grubOut)
		if err := grubOut.Close(); err != nil {
			return nil, err
		}

		cmd := exec.Command("grub-mkstandalone",
			"--format="+format,
			"--output="+grubOut.Name(),
			"boot/grub/grub.cfg="+grubCfg.Name(),
			"current/vmlinuz="+kernelPath,
			"current/initrd="+initrdPath,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		return ioutil.ReadFile(grubOut.Name())
	}

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

	ba, err := grubMk("x86_64-efi")
	if err != nil {
		return err
	}

	err = archAdd("EFI/boot/bootx64.efi", ba)
	if err != nil {
		return
	}

	if false {
		// TODO
		ba, err = grubMk("i386-pc")
		if err != nil {
			return err
		}

		arch.WriteHeader(&tar.Header{
			Name: "grub.img",
			Size: int64(len(ba)),
		})
		arch.Write(ba)
	}

	// config
	cfgBytes, cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	archAdd("config.yaml", cfgBytes)

	// kernel and initrd
	type distCopy struct {
		Src []string
		Dst string
	}

	copies := []distCopy{
		// XXX {Src: []string{"kernels", ctx.Group.Kernel}, Dst: "current/vmlinuz"},
		// XXX {Src: []string{"initrd", ctx.Group.Initrd}, Dst: "current/initrd"},
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
