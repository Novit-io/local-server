package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func buildBootISO(out io.Writer, ctx *renderContext) error {
	tempDir, err := ioutil.TempDir("/tmp", "iso-")
	if err != nil {
		return err
	}

	defer os.RemoveAll(tempDir)

	cp := func(src, dst string) error {
		log.Printf("iso: adding %s as %s", src, dst)
		in, err := os.Open(src)
		if err != nil {
			return err
		}

		defer in.Close()

		outPath := filepath.Join(tempDir, dst)

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}

		out, err := os.Create(outPath)
		if err != nil {
			return err
		}

		defer out.Close()

		_, err = io.Copy(out, in)
		return err
	}

	err = func() error {
		// grub

		if err := os.MkdirAll(filepath.Join(tempDir, "grub"), 0755); err != nil {
			return err
		}
		err = ioutil.WriteFile(filepath.Join(tempDir, "grub", "grub.cfg"), []byte(`
search --set=root --file /config.yaml

insmod all_video
set timeout=3

menuentry "Direktil" {
    linux  /vmlinuz direktil.boot=DEVNAME=sr0 direktil.boot.fs=iso9660
    initrd /initrd
}
`), 0644)
		if err != nil {
			return err
		}

		coreImgPath := filepath.Join(tempDir, "grub", "core.img")
		grubCfgPath := filepath.Join(tempDir, "grub", "grub.cfg")

		cmd := exec.Command("grub-mkstandalone",
			"--format=i386-pc",
			"--output="+coreImgPath,
			"--install-modules=linux normal iso9660 biosdisk memdisk search tar ls",
			"--modules=linux normal iso9660 biosdisk search",
			"--locales=",
			"--fonts=",
			"boot/grub/grub.cfg="+grubCfgPath,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}

		defer os.Remove(coreImgPath)
		defer os.Remove(grubCfgPath)

		out, err := os.Create(filepath.Join(tempDir, "grub", "bios.img"))
		if err != nil {
			return err
		}

		defer out.Close()

		b, err := ioutil.ReadFile("/usr/lib/grub/i386-pc/cdboot.img")
		if err != nil {
			return err
		}

		if _, err := out.Write(b); err != nil {
			return err
		}

		b, err = ioutil.ReadFile(coreImgPath)
		if err != nil {
			return err
		}

		if _, err := out.Write(b); err != nil {
			return err
		}

		return nil
	}()
	if err != nil {
		return err
	}

	// config
	cfgBytes, cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	ioutil.WriteFile(filepath.Join(tempDir, "config.yaml"), cfgBytes, 0600)

	// kernel and initrd
	type distCopy struct {
		Src []string
		Dst string
	}

	copies := []distCopy{
		{Src: []string{"kernels", ctx.Group.Kernel}, Dst: "vmlinuz"},
		{Src: []string{"initrd", ctx.Group.Initrd}, Dst: "initrd"},
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

		err = cp(outPath, copy.Dst)
		if err != nil {
			return err
		}
	}

	// build the ISO
	mkisofs, err := exec.LookPath("genisoimage")
	if err != nil {
		mkisofs, err = exec.LookPath("mkisofs")
	}
	if err != nil {
		return err
	}

	cmd := exec.Command(mkisofs,
		"-quiet",
		"-joliet",
		"-joliet-long",
		"-rock",
		"-translation-table",
		"-no-emul-boot",
		"-boot-load-size", "4",
		"-boot-info-table",
		"-eltorito-boot", "grub/bios.img",
		"-eltorito-catalog", "grub/boot.cat",
		tempDir,
	)
	cmd.Stdout = out
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
