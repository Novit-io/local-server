package main

import (
	"net/http"

	restful "github.com/emicklei/go-restful"
)

func wsListHosts(req *restful.Request, resp *restful.Response) {
	cfg, err := readConfig()
	if err != nil {
		resp.WriteErrorString(http.StatusServiceUnavailable, "failed to read configuration")
		return
	}

	names := make([]string, len(cfg.Hosts))
	for i, host := range cfg.Hosts {
		names[i] = host.Name
	}

	resp.WriteEntity(names)
}
