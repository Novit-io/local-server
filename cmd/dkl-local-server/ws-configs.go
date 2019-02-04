package main

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	restful "github.com/emicklei/go-restful"
)

func wsUploadConfig(req *restful.Request, resp *restful.Response) {
	r := req.Request
	w := resp.ResponseWriter

	if !authorizeAdmin(r) {
		forbidden(w, r)
		return
	}

	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}

	out, err := ioutil.TempFile(*dataDir, ".config-upload")
	if err != nil {
		writeError(w, err)
		return
	}

	defer os.Remove(out.Name())

	_, err = io.Copy(out, r.Body)
	out.Close()
	if err != nil {
		writeError(w, err)
		return
	}

	archivesPath := filepath.Join(*dataDir, "archives")
	cfgPath := configFilePath()

	err = os.MkdirAll(archivesPath, 0700)
	if err != nil {
		writeError(w, err)
		return
	}

	err = func() (err error) {
		backupPath := filepath.Join(archivesPath, "config."+ulid()+".yaml.gz")

		bck, err := os.Create(backupPath)
		if err != nil {
			return
		}

		defer bck.Close()

		in, err := os.Open(cfgPath)
		if err != nil {
			return
		}

		gz, err := gzip.NewWriterLevel(bck, 2)
		if err != nil {
			return
		}

		_, err = io.Copy(gz, in)
		gz.Close()
		in.Close()
		return
	}()

	if err != nil {
		writeError(w, err)
		return
	}

	err = os.Rename(out.Name(), cfgPath)
	if err != nil {
		writeError(w, err)
		return
	}
}
