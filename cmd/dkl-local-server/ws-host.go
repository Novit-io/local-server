package main

import (
	"log"
	"net/http"
	"path"

	restful "github.com/emicklei/go-restful"
)

type wsHost struct {
	prefix  string
	getHost func(req *restful.Request) string
}

func (ws *wsHost) register(rws *restful.WebService) {
	for _, what := range []string{
		"boot.img",
		"boot.img.gz",
		"boot.img.lz4",
		"boot.iso",
		"boot.tar",
		"config",
		"initrd",
		"ipxe",
		"kernel",
	} {
		rws.Route(rws.GET(ws.prefix + "/" + what).To(ws.render))
	}
}

func (ws *wsHost) render(req *restful.Request, resp *restful.Response) {
	hostname := ws.getHost(req)
	if hostname == "" {
		http.NotFound(resp.ResponseWriter, req.Request)
		return
	}

	cfg, err := readConfig()
	if err != nil {
		writeError(resp.ResponseWriter, err)
		return
	}

	host := cfg.Host(hostname)
	if host == nil {
		log.Print("no host named ", hostname)
		http.NotFound(resp.ResponseWriter, req.Request)
		return
	}

	what := path.Base(req.Request.URL.Path)

	renderHost(resp.ResponseWriter, req.Request, what, host, cfg)
}
