package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

var (
	secrets SecretBackend
)

type SecretBackend interface {
	Get(ref string) (string, error)
	Set(ref, value string) error
}

type SecretsFile struct {
	Path string
}

func (sf *SecretsFile) readData() (map[string]string, error) {
	ba, err := ioutil.ReadFile(sf.Path)
	if err != nil {
		return nil, err
	}

	data := map[string]string{}
	yaml.Unmarshal(ba, &data)

	return data, nil
}

func (sf *SecretsFile) Get(ref string) (string, error) {
	data, err := sf.readData()

	if os.IsNotExist(err) {
		return "", nil

	} else if err != nil {
		log.Printf("secret file: failed to read: %v", err)
		return "", err
	}

	return data[ref], nil
}

func (sf *SecretsFile) Set(ref, value string) (err error) {
	data, err := sf.readData()

	if os.IsNotExist(err) {
		data = map[string]string{}

	} else if err != nil {
		log.Printf("secret file: failed to read: %v", err)
		return
	}

	data[ref] = value

	ba, err := yaml.Marshal(data)
	if err != nil {
		log.Printf("secret file: failed to encode: %v", err)
		return
	}

	os.Rename(sf.Path, sf.Path+".old")

	err = ioutil.WriteFile(sf.Path, ba, 0600)
	if err != nil {
		log.Printf("secret file: failed to write: %v", err)
		return
	}

	return
}

func getSecret(ref string, ctx *renderContext) (string, error) {
	fullRef := fmt.Sprintf("%s/%s", ctx.Cluster.Name, ref)

	v, err := secrets.Get(fullRef)

	if err != nil {
		return "", err
	}

	if v != "" {
		return v, nil
	}

	// no value, generate
	split := strings.SplitN(ref, ":", 2)
	kind, path := split[0], split[1]

	switch kind {
	case "tls-key":
		_, ba := PrivateKeyPEM()
		v = string(ba)

	case "tls-self-signed-cert":
		caKey, err := loadPrivateKey(path, ctx)
		if err != nil {
			return "", err
		}

		ba := SelfSignedCertificatePEM(5, caKey)
		v = string(ba)

	case "tls-host-cert":
		hostKey, err := loadPrivateKey(path, ctx)
		if err != nil {
			return "", err
		}

		ba, err := HostCertificatePEM(3, hostKey, ctx)
		if err != nil {
			return "", err
		}
		v = string(ba)

	default:
		return "", fmt.Errorf("unknown secret kind: %q", kind)
	}

	if v == "" {
		panic("value not generated?!")
	}

	if err := secrets.Set(fullRef, v); err != nil {
		return "", err
	}

	return v, nil
}
