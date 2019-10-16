package main

import (
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	bind        = flag.String("bind", ":8080", "Bind address")
	uploadToken = flag.String("upload-token", "", "Upload token (no uploads allowed if empty)")
	storeDir    = flag.String("store-dir", "/srv/dkl-store", "Store directory")
)

func main() {
	flag.Parse()

	http.HandleFunc("/", handleHTTP)

	log.Print("listening on ", *bind)
	http.ListenAndServe(*bind, nil)
}

func handleHTTP(w http.ResponseWriter, req *http.Request) {
	filePath := filepath.Join(*storeDir, req.URL.Path)

	l := fmt.Sprintf("%s %s", req.Method, filePath)
	log.Print(l)
	defer log.Print(l, " done")

	stat, err := os.Stat(filePath)
	if err != nil {
		writeErr(err, w)
		return
	}

	if stat.Mode().IsDir() {
		http.NotFound(w, req)
		return
	}

	switch req.Method {
	case "GET":
		sha1Hex, err := hashOf(filePath)
		if err != nil {
			writeErr(err, w)
			return
		}

		w.Header().Set("X-Content-SHA1", sha1Hex)
		http.ServeFile(w, req, filePath)

	case "POST":
		// TODO upload
		http.Error(w, "not implemented", http.StatusNotImplemented)

	default:
		http.NotFound(w, req)
		return
	}
}

func writeErr(err error, w http.ResponseWriter) {
	if os.IsNotExist(err) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not found\n"))
		return
	}

	log.Print("internal error: ", err)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Internal error\n"))
}

func hashOf(filePath string) (sha1Hex string, err error) {
	sha1Path := filePath + ".sha1"

	fileStat, err := os.Stat(filePath)
	if err != nil {
		return
	}

	sha1Stat, err := os.Stat(sha1Path)

	if err == nil {
		if sha1Stat.ModTime().After(fileStat.ModTime()) {
			// cached value is up-to-date
			sha1HexBytes, readErr := ioutil.ReadFile(sha1Path)

			if readErr == nil {
				sha1Hex = string(sha1HexBytes)
				return
			}
		}
	} else if !os.IsNotExist(err) {
		// failed to stat cached value
		return
	}

	// no cached value could be read
	log.Print("hashing ", filePath)
	start := time.Now()

	// hash the input
	f, err := os.Open(filePath)
	if err != nil {
		return
	}

	defer f.Close()

	h := sha1.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return
	}

	sha1Hex = hex.EncodeToString(h.Sum(nil))

	log.Print("hashing ", filePath, " took ", time.Since(start).Truncate(time.Millisecond))

	if writeErr := ioutil.WriteFile(sha1Path, []byte(sha1Hex), 0644); writeErr != nil {
		log.Printf("WARNING: failed to cache SHA1: %v", writeErr)
	}

	return
}
