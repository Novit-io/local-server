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
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()

	http.HandleFunc("/", handleHTTP)

	log.Print("listening on ", *bind)
	log.Fatal(http.ListenAndServe(*bind, nil))
}

func handleHTTP(w http.ResponseWriter, req *http.Request) {
	filePath := filepath.Join(*storeDir, req.URL.Path)

	l := fmt.Sprintf("%s %s", req.Method, filePath)
	log.Print(l)
	defer log.Print(l, " done")

	stat, err := os.Stat(filePath)
	if err != nil && !os.IsNotExist(err) {
		writeErr(err, w)
		return
	} else if err == nil && stat.Mode().IsDir() {
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
		tmpOut := filepath.Join(filepath.Dir(filePath), "."+filepath.Base(filePath))

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			writeErr(err, w)
			return
		}

		out, err := os.Create(tmpOut)
		if err != nil {
			writeErr(err, w)
			return
		}

		h := sha1.New()
		mw := io.MultiWriter(out, h)

		_, err = io.Copy(mw, req.Body)
		out.Close()

		if err != nil {
			os.Remove(tmpOut)

			writeErr(err, w)
			return
		}

		sha1Hex := hex.EncodeToString(h.Sum(nil))
		log.Print("upload SHA1: ", sha1Hex)

		reqSHA1 := req.Header.Get("X-Content-SHA1")
		if reqSHA1 != "" && reqSHA1 != sha1Hex {
			err = fmt.Errorf("upload SHA1 does not match given SHA1: %s", reqSHA1)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error() + "\n"))
			return
		}

		os.Rename(tmpOut, filePath)

		if err := ioutil.WriteFile(filePath+".sha1", []byte(sha1Hex), 0644); err != nil {
			writeErr(err, w)
			return
		}

		w.WriteHeader(http.StatusCreated)

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

	log.Output(2, fmt.Sprint("internal error: ", err))
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
