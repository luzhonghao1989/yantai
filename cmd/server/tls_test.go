package main

import (
	"os"
	"testing"
)

func TestSyncCertificatePersistsAndDoesNotRotate(t *testing.T) {
	root := t.TempDir()
	cert, key, err := ensureSyncCertificate(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := certificateFingerprint(cert, key)
	if err != nil || len(first) != 64 {
		t.Fatal("bad certificate", err)
	}
	cert2, key2, err := ensureSyncCertificate(root)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := certificateFingerprint(cert2, key2)
	if first != second {
		t.Fatal("identity changed on restart")
	}
	if err := os.Remove(cert); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ensureSyncCertificate(root); err == nil {
		t.Fatal("partial certificate silently rotated")
	}
}
func TestWebListenerMustBeLocal(t *testing.T) {
	for _, address := range []string{"127.0.0.1:48121", "[::1]:48121", "localhost:48121"} {
		if !loopbackAddress(address) {
			t.Fatal(address)
		}
	}
	for _, address := range []string{"0.0.0.0:48121", ":48121", "192.168.0.1:48121", "server.example:48121", "invalid"} {
		if loopbackAddress(address) {
			t.Fatal(address)
		}
	}
}
