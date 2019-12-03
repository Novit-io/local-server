package main

import "testing"

func init() {
	DontSave = true
}

func TestSSHKeyGet(t *testing.T) {
	sd := &SecretData{
		clusters: make(map[string]*ClusterSecrets),
	}

	if _, err := sd.SSHKeyPairs("test", "host"); err != nil {
		t.Error(err)
	}
}
