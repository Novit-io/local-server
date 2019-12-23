package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pierrec/lz4"
)

func buildBootImg(out io.Writer, ctx *renderContext) (err error) {
	bootImg, err := ioutil.TempFile(os.TempDir(), "boot.img-")
	if err != nil {
		return
	}
	defer rmTempFile(bootImg)

	err = setupBootImage(bootImg, ctx)
	if err != nil {
		return
	}

	// send the result
	bootImg.Seek(0, os.SEEK_SET)
	io.Copy(out, bootImg)
	return
}

func buildBootImgLZ4(out io.Writer, ctx *renderContext) (err error) {
	lz4Out := lz4.NewWriter(out)

	if err = buildBootImg(lz4Out, ctx); err != nil {
		return
	}

	lz4Out.Close()
	return
}

func buildBootImgGZ(out io.Writer, ctx *renderContext) (err error) {
	gzOut := gzip.NewWriter(out)

	if err = buildBootImg(gzOut, ctx); err != nil {
		return
	}

	gzOut.Close()
	return
}

func setupBootImage(bootImg *os.File, ctx *renderContext) (err error) {
	path, err := ctx.distFetch("grub-support", "1.0.0")
	if err != nil {
		return
	}

	baseImage, err := os.Open(path)
	if err != nil {
		return
	}

	defer baseImage.Close()

	baseImageGz, err := gzip.NewReader(baseImage)
	if err != nil {
		return
	}

	defer baseImageGz.Close()
	_, err = io.Copy(bootImg, baseImageGz)

	if err != nil {
		return
	}

	log.Print("running losetup...")
	cmd := exec.Command("losetup", "--find", "--show", "--partscan", bootImg.Name())
	cmd.Stderr = os.Stderr
	devb, err := cmd.Output()
	if err != nil {
		return
	}

	dev := strings.TrimSpace(string(devb))
	defer func() {
		log.Print("detaching ", dev)
		run("losetup", "-d", dev)
	}()

	log.Print("device: ", dev)

	tempDir := bootImg.Name() + ".p1.mount"

	err = os.Mkdir(tempDir, 0755)
	if err != nil {
		return
	}

	defer func() {
		log.Print("removing ", tempDir)
		os.RemoveAll(tempDir)
	}()

	err = syscall.Mount(dev+"p1", tempDir, "vfat", 0, "")
	if err != nil {
		return fmt.Errorf("failed to mount %s to %s: %v", dev+"p1", tempDir, err)
	}

	defer func() {
		log.Print("unmounting ", tempDir)
		syscall.Unmount(tempDir, 0)
	}()

	// add system elements
	tarOut, tarIn := io.Pipe()
	go func() {
		err2 := buildBootTar(tarIn, ctx)
		tarIn.Close()
		if err2 != nil {
			err = err2
		}
	}()

	defer tarOut.Close()

	tarRd := tar.NewReader(tarOut)

	for {
		hdr, err := tarRd.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		log.Print("tar: extracting ", hdr.Name)

		outPath := filepath.Join(tempDir, hdr.Name)
		os.MkdirAll(filepath.Dir(outPath), 0755)

		f, err := os.Create(outPath)
		if err != nil {
			return err
		}

		_, err = io.Copy(f, tarRd)
		f.Close()

		if err != nil {
			return err
		}
	}

	return
}

func run(program string, args ...string) (err error) {
	cmd := exec.Command(program, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
