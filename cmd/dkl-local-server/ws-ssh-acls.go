package main

import (
	"net/http"
	"os"
	"path/filepath"

	restful "github.com/emicklei/go-restful"
	yaml "gopkg.in/yaml.v2"
)

type SSH_ACL struct {
	Keys     []string
	Clusters []string
	Groups   []string
	Hosts    []string
}

func loadSSH_ACLs() (acls []SSH_ACL, err error) {
	f, err := os.Open(filepath.Join(*dataDir, "ssh-acls.yaml"))
	if err != nil {
		return
	}

	defer f.Close()

	err = yaml.NewDecoder(f).Decode(&acls)
	return
}

func wsSSH_ACL_List(req *restful.Request, resp *restful.Response) {
	// TODO
	http.NotFound(resp.ResponseWriter, req.Request)
}

func wsSSH_ACL_Get(req *restful.Request, resp *restful.Response) {
	// TODO
	http.NotFound(resp.ResponseWriter, req.Request)
}

func wsSSH_ACL_Set(req *restful.Request, resp *restful.Response) {
	// TODO
	http.NotFound(resp.ResponseWriter, req.Request)
}
