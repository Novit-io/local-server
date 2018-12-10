package main

import (
	"log"
	"net/http"
)

func renderKernel(w http.ResponseWriter, r *http.Request, ctx *renderContext) error {
	path, err := ctx.distFetch("kernels", ctx.Host.Kernel)
	if err != nil {
		return err
	}

	log.Printf("sending kernel %s for %q", ctx.Host.Kernel, ctx.Host.Name)
	http.ServeFile(w, r, path)
	return nil
}
