package main

import (
	"bytes"
	"io"
	"log"
	"text/template"
)

func renderIPXE(out io.Writer, ctx *renderContext) error {
	log.Printf("sending IPXE code for %q", ctx.Host.Name)

	tmpl, err := template.New("ipxe").Parse(ctx.Group.IPXE)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	if err := tmpl.Execute(buf, ctx.asMap()); err != nil {
		return err
	}

	_, err = buf.WriteTo(out)
	return err
}
