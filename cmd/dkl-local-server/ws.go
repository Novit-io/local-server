package main

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/emicklei/go-restful"
)

func buildWS() *restful.WebService {
	ws := &restful.WebService{}

	ws.Route(ws.POST("/configs").To(wsUploadConfig))

	(&wsHost{
		prefix:  "",
		getHost: detectHost,
	}).register(ws)

	(&wsHost{
		prefix: "/hosts/{hostname}",
		getHost: func(req *restful.Request) string {
			return req.PathParameter("hostname")
		},
	}).register(ws)

	return ws
}

func detectHost(req *restful.Request) string {
	r := req.Request
	remoteAddr := r.RemoteAddr

	if *trustXFF {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			remoteAddr = strings.Split(xff, ",")[0]
		}
	}

	hostIP, _, err := net.SplitHostPort(remoteAddr)

	if err != nil {
		hostIP = remoteAddr
	}

	cfg, err := readConfig()
	if err != nil {
		return ""
	}

	host := cfg.HostByIP(hostIP)

	if host == nil {
		log.Print("no host found for IP ", hostIP)
		return ""
	}

	return host.Name
}

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
