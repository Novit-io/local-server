package main

import (
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/emicklei/go-restful"
	"novit.nc/direktil/local-server/pkg/mime"
	"novit.nc/direktil/pkg/localconfig"
)

func buildWS() *restful.WebService {
	ws := &restful.WebService{}

	// configs API
	ws.Route(ws.POST("/configs").Filter(adminAuth).To(wsUploadConfig).
		Doc("Upload a new current configuration, archiving the previous one"))

	// clusters API
	ws.Route(ws.GET("/clusters").Filter(adminAuth).To(wsListClusters).
		Doc("List clusters"))

	ws.Route(ws.GET("/clusters/{cluster-name}").Filter(adminAuth).To(wsCluster).
		Doc("Get cluster details"))

	ws.Route(ws.GET("/clusters/{cluster-name}/addons").Filter(adminAuth).To(wsClusterAddons).
		Produces(mime.YAML).
		Doc("Get cluster addons").
		Returns(http.StatusOK, "OK", nil).
		Returns(http.StatusNotFound, "The cluster does not exists or does not have addons defined", nil))

	ws.Route(ws.GET("/clusters/{cluster-name}/passwords").Filter(adminAuth).To(wsClusterPasswords).
		Doc("List cluster's passwords"))
	ws.Route(ws.GET("/clusters/{cluster-name}/passwords/{password-name}").Filter(adminAuth).To(wsClusterPassword).
		Doc("Get cluster's password"))
	ws.Route(ws.PUT("/clusters/{cluster-name}/passwords/{password-name}").Filter(adminAuth).To(wsClusterSetPassword).
		Doc("Set cluster's password"))

	// hosts API
	ws.Route(ws.GET("/hosts").Filter(hostsAuth).To(wsListHosts).
		Doc("List hosts"))

	(&wsHost{
		prefix:  "/me",
		hostDoc: "detected host",
		getHost: detectHost,
	}).register(ws, func(rb *restful.RouteBuilder) {
		rb.Notes("In this case, the host is detected from the remote IP")
	})

	(&wsHost{
		prefix:  "/hosts/{host-name}",
		hostDoc: "given host",
		getHost: func(req *restful.Request) string {
			return req.PathParameter("host-name")
		},
	}).register(ws, func(rb *restful.RouteBuilder) {
		rb.Filter(adminAuth)
	})

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

func wsReadConfig(resp *restful.Response) *localconfig.Config {
	cfg, err := readConfig()
	if err != nil {
		log.Print("failed to read config: ", err)
		resp.WriteErrorString(http.StatusServiceUnavailable, "failed to read config")
		return nil
	}

	return cfg
}

func wsNotFound(req *restful.Request, resp *restful.Response) {
	http.NotFound(resp.ResponseWriter, req.Request)
}

func wsError(resp *restful.Response, err error) {
	log.Print("request failed: ", err)
	resp.WriteErrorString(
		http.StatusInternalServerError,
		http.StatusText(http.StatusInternalServerError))
}
