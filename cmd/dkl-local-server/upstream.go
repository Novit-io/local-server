package main

import (
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	gopath "path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/miolini/datacounter"
)

var (
	upstreamURL = flag.String("upstream", "https://direktil.novit.nc/dist", "Upstream server for dist elements")
)

func (ctx *renderContext) distFetch(path ...string) (outPath string, err error) {
	outPath = ctx.distFilePath(path...)

	if _, err = os.Stat(outPath); err == nil {
		return
	} else if !os.IsNotExist(err) {
		return
	}

	subPath := gopath.Join(path...)

	log.Print("need to fetch ", subPath)

	if err = os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return
	}

	fullURL := *upstreamURL + "/" + subPath

	resp, err := http.Get(fullURL)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err = fmt.Errorf("wrong status: %s", resp.Status)
		return
	}

	length, _ := strconv.Atoi(resp.Header.Get("Content-Length"))

	fOut, err := os.Create(filepath.Join(filepath.Dir(outPath), "._part_"+filepath.Base(outPath)))
	if err != nil {
		return
	}

	hash := sha1.New()

	body := datacounter.NewReaderCounter(resp.Body)
	out := io.MultiWriter(fOut, hash)

	done := make(chan error, 1)
	go func() {
		_, err = io.Copy(out, body)
		fOut.Close()

		if err != nil {
			os.Remove(fOut.Name())
		}

		done <- err
		close(done)
	}()

	start := time.Now()

wait:
	select {
	case <-time.After(10 * time.Second):
		status := ""
		if length != 0 {
			count := body.Count()
			elapsedDuration := time.Since(start)

			progress := float64(count) / float64(length)

			elapsed := float64(elapsedDuration)
			remaining := time.Duration(elapsed/progress - elapsed)

			status = fmt.Sprintf(" (%.2f%%, ETA %v, %s/s)",
				progress*100,
				remaining.Truncate(time.Second),
				humanize.Bytes(uint64(float64(count)/elapsedDuration.Seconds())))
		}
		log.Printf("still fetching %s%s...", subPath, status)
		goto wait

	case err = <-done:
		if err != nil {
			log.Print("fetch of ", subPath, " failed: ", err)
			return
		}
	}

	hexSum := hex.EncodeToString(hash.Sum(nil))
	log.Printf("fetch of %s finished (SHA1 checksum: %s)", subPath, hexSum)

	if remoteSum := resp.Header.Get("X-Content-SHA1"); remoteSum != "" {
		log.Printf("fetch of %s: remote SHA1 checksum: %s", subPath, remoteSum)
		if remoteSum != hexSum {
			err = fmt.Errorf("wrong SHA1 checksum: server=%s local=%s", remoteSum, hexSum)
			log.Print("fetch of ", subPath, ": ", err)
			os.Remove(fOut.Name())
			return
		}
	}

	err = os.Rename(fOut.Name(), outPath)

	return
}
