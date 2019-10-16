package main

import (
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

var (
	token = flag.String("token", "", "Upload token")
)

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) != 2 {
		fmt.Print("source file and target URL are required")
		os.Exit(1)
	}

	inPath := args[0]
	outURL := args[1]

	in, err := os.Open(inPath)
	fail(err)

	// hash the file
	log.Print("hashing...")
	h := sha1.New()
	_, err = io.Copy(h, in)
	fail(err)

	sha1Hex := hex.EncodeToString(h.Sum(nil))

	log.Print("SHA1 of source: ", sha1Hex)

	// rewind
	_, err = in.Seek(0, os.SEEK_SET)
	fail(err)

	// upload
	req, err := http.NewRequest("POST", outURL, in)
	fail(err)

	req.Header.Set("X-Content-SHA1", sha1Hex)

	log.Print("uploading...")
	resp, err := http.DefaultClient.Do(req)
	fail(err)

	if resp.StatusCode != http.StatusCreated {
		log.Fatalf("unexpected HTTP status: %s", resp.Status)
	}

	log.Print("uploaded successfully")
}

func fail(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
