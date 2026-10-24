package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func certificateFingerprint(cert, key string) (string, error) {
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(pair.Certificate[0])
	return hex.EncodeToString(digest[:]), nil
}

func ensureSyncCertificate(root string) (string, string, error) {
	dir := filepath.Join(root, "tls")
	cert, key := filepath.Join(dir, "sync-cert.pem"), filepath.Join(dir, "sync-key.pem")
	_, ce := os.Stat(cert)
	_, ke := os.Stat(key)
	if ce == nil && ke == nil {
		_, err := certificateFingerprint(cert, key)
		return cert, key, err
	}
	// Never silently rotate an existing identity, including partial/corrupt files.
	if !os.IsNotExist(ce) || !os.IsNotExist(ke) {
		return "", "", errors.New("同步证书不完整或无法读取，请检查 data/tls，勿自动替换已有证书")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Yantai private sync"},
		NotBefore: now.Add(-5 * time.Minute), NotAfter: now.AddDate(5, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}
	write := func(path, kind string, data []byte) error {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		err = pem.Encode(file, &pem.Block{Type: kind, Bytes: data})
		closeErr := file.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	if err = write(key, "PRIVATE KEY", keyDER); err == nil {
		err = write(cert, "CERTIFICATE", der)
	}
	return cert, key, err
}
