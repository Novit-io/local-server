package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	gopath "path"
	"path/filepath"
	"time"
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

	if resp.StatusCode != 200 {
		err = fmt.Errorf("wrong status: %s", resp.Status)
		resp.Body.Close()
		return
	}

	tempOutPath := filepath.Join(filepath.Dir(outPath), "._part_"+filepath.Base(outPath))

	done := make(chan error, 1)
	go func() {
		defer resp.Body.Close()
		defer close(done)

		out, err := os.Create(tempOutPath)
		if err != nil {
			done <- err
			return
		}

		defer out.Close()

		_, err = io.Copy(out, resp.Body)
		done <- err
	}()

wait:
	select {
	case <-time.After(10 * time.Second):
		log.Print("still fetching ", subPath, "...")
		goto wait

	case err = <-done:
		if err != nil {
			log.Print("fetch of ", subPath, " failed: ", err)
			os.Remove(tempOutPath)
			return
		}

		log.Print("fetch of ", subPath, " finished")
	}

	// TODO checksum

	err = os.Rename(tempOutPath, outPath)

	return
}
