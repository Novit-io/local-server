package main

import (
	"archive/tar"
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

	// 2MB + 2GB + 2MB + 34 sectors
	bootImg.Truncate(2<<30 + 4<<20 + 34*512)

	// partition
	err = run("sgdisk",
		"--new=0:4096:+2G", "--typecode=0:EF00", "-c", "0:boot",
		"--new=0:0:+2M", "--typecode=0:EF02", "-c", "0:BIOS-BOOT",
		"--hybrid=1:2", "--print", bootImg.Name())
	if err != nil {
		return
	}

	err = setupBootImage(bootImg, ctx)
	if err != nil {
		return
	}

	// send the result
	bootImg.Seek(0, os.SEEK_SET)

	lz4Out := lz4.NewWriter(out)
	io.Copy(lz4Out, bootImg)
	lz4Out.Close()

	return
}

func setupBootImage(bootImg *os.File, ctx *renderContext) (err error) {
	devb, err := exec.Command("losetup", "--find", "--show", "--partscan", bootImg.Name()).CombinedOutput()
	if err != nil {
		return
	}

	dev := strings.TrimSpace(string(devb))
	defer run("losetup", "-d", dev)

	log.Print("device: ", dev)

	err = run("mkfs.vfat", "-n", "DKLBOOT", dev+"p1")
	if err != nil {
		return
	}

	tempDir := bootImg.Name() + ".p1.mount"

	err = os.Mkdir(tempDir, 0755)
	if err != nil {
		return
	}

	defer func() {
		log.Print("Removing ", tempDir)
		os.RemoveAll(tempDir)
	}()

	err = syscall.Mount(dev+"p1", tempDir, "vfat", 0, "")
	if err != nil {
		return
	}

	defer func() {
		log.Print("Unmounting ", tempDir)
		syscall.Unmount(tempDir, 0)
	}()

	// setup grub
	err = run("/scripts/grub_install.sh", bootImg.Name(), dev, tempDir)
	if err != nil {
		return
	}

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
