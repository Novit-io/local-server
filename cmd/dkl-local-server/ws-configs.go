package main

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	restful "github.com/emicklei/go-restful"
)

func wsUploadConfig(req *restful.Request, resp *restful.Response) {
	body := req.Request.Body

	err := writeNewConfig(body)
	body.Close()

	if err != nil {
		wsError(resp, err)
	}
}

func writeNewConfig(reader io.Reader) (err error) {
	out, err := ioutil.TempFile(*dataDir, ".config-upload")
	if err != nil {
		return
	}

	defer os.Remove(out.Name())

	_, err = io.Copy(out, reader)
	out.Close()
	if err != nil {
		return
	}

	cfgPath := configFilePath()
	in, err := os.Open(cfgPath)

	if err == nil {
		err = backupCurrentConfig(in)
	} else if !os.IsNotExist(err) {
		return
	}

	err = os.Rename(out.Name(), cfgPath)
	return
}

func backupCurrentConfig(in io.ReadCloser) (err error) {
	archivesPath := filepath.Join(*dataDir, "archives")

	err = os.MkdirAll(archivesPath, 0700)
	if err != nil {
		return
	}

	backupPath := filepath.Join(archivesPath, "config."+ulid()+".yaml.gz")

	bck, err := os.Create(backupPath)
	if err != nil {
		return
	}

	defer bck.Close()

	gz, err := gzip.NewWriterLevel(bck, 2)
	if err != nil {
		return
	}

	_, err = io.Copy(gz, in)
	gz.Close()
	in.Close()

	return
}
