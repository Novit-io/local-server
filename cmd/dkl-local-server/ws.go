package main

import (
	"log"
	"net"
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

func wsUploadConfig(req *restful.Request, res *restful.Response) {
	// TODO
}
