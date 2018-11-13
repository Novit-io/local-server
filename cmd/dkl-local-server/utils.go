package main

import (
	"io"
	"os"
)

func writeFile(out io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = io.Copy(out, f)
	return err
}
