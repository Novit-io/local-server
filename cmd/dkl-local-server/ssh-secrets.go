package main

import (
	"crypto/dsa"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
)

type SSHKeyPair struct {
	Type    string
	Public  string
	Private string
}

func (sd *SecretData) SSHKeyPairs(cluster, host string) (pairs []SSHKeyPair, err error) {
	cs := sd.cluster(cluster)

	if cs.SSHKeyPairs == nil {
		cs.SSHKeyPairs = map[string][]SSHKeyPair{}
	}

	outFile, err := ioutil.TempFile("/tmp", "dls-key.")
	if err != nil {
		return
	}

	outPath := outFile.Name()

	removeTemp := func() {
		os.Remove(outPath)
		os.Remove(outPath + ".pub")
	}

	defer removeTemp()

	pairs = cs.SSHKeyPairs[host]

	didGenerate := false

genLoop:
	for _, keyType := range []string{
		"rsa",
		"dsa",
		"ecdsa",
		"ed25519",
	} {
		for _, pair := range pairs {
			if pair.Type == keyType {
				continue genLoop
			}
		}

		didGenerate = true

		removeTemp()

		var out, privKey, pubKey []byte

		out, err = exec.Command("ssh-keygen",
			"-N", "",
			"-C", "root@"+host,
			"-f", outPath,
			"-t", keyType).CombinedOutput()
		if err != nil {
			err = fmt.Errorf("ssh-keygen failed: %v: %s", err, string(out))
			return
		}

		privKey, err = ioutil.ReadFile(outPath)
		if err != nil {
			return
		}

		os.Remove(outPath)

		pubKey, err = ioutil.ReadFile(outPath + ".pub")
		if err != nil {
			return
		}

		os.Remove(outPath + ".pub")

		pairs = append(pairs, SSHKeyPair{
			Type:    keyType,
			Public:  string(pubKey),
			Private: string(privKey),
		})
	}

	if didGenerate {
		cs.SSHKeyPairs[host] = pairs
		err = sd.Save()
	}

	return
}

func sshKeyGenDSA() (data []byte, pubKey interface{}, err error) {
	privKey := &dsa.PrivateKey{}

	err = dsa.GenerateParameters(&privKey.Parameters, rand.Reader, dsa.L1024N160)
	if err != nil {
		return
	}

	err = dsa.GenerateKey(privKey, rand.Reader)
	if err != nil {
		return
	}

	data, err = asn1.Marshal(*privKey)
	//data, err = x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return
	}

	pubKey = privKey.PublicKey
	return
}

func sshKeyGenRSA() (data []byte, pubKey interface{}, err error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return
	}

	data = x509.MarshalPKCS1PrivateKey(privKey)
	pubKey = privKey.Public()

	return
}

func sshKeyGenECDSA() (data []byte, pubKey interface{}, err error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return
	}

	data, err = x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return
	}

	pubKey = privKey.Public()

	return
}

func sshKeyGenED25519() (data []byte, pubKey interface{}, err error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)

	data, err = x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return
	}

	return
}
