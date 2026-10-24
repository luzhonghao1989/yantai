package workspace

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
)

func syncHTTPClient(cfg SyncSettings) (*http.Client, error) {
	if err := validateSyncSettings(&cfg); err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.TLSFingerprint != "" {
		pin, _ := hex.DecodeString(cfg.TLSFingerprint)
		// Explicit certificate pin replaces CA/hostname validation for private certificates.
		// Never accept a peer without checking the configured digest and certificate validity.
		transport.TLSClientConfig.InsecureSkipVerify = true
		transport.TLSClientConfig.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("服务端未提供证书")
			}
			cert := cs.PeerCertificates[0]
			digest := sha256.Sum256(cert.Raw)
			if subtle.ConstantTimeCompare(pin, digest[:]) != 1 {
				return errors.New("服务端证书指纹不匹配")
			}
			now := time.Now()
			if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
				return errors.New("服务端证书不在有效期内")
			}
			return nil
		}
	}
	return &http.Client{Transport: transport, Timeout: 2 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

// SyncHandler exposes only machine-to-machine sync. No static assets, login,
// cookies, model configuration, agent endpoints or project editor API exist here.
func (s *Server) SyncHandler() http.Handler {
	mux := http.NewServeMux()
	s.registerSyncRemote(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		reject := func() { w.WriteHeader(http.StatusNotFound) }
		// Do not accept browser cookies or cross-origin/browser requests.
		if !s.Remote || r.TLS == nil || r.Header.Get("Origin") != "" || len(s.AuthKey) < 16 ||
			subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.AuthKey)) != 1 {
			reject()
			return
		}
		path := r.URL.Path
		allowed := path == "/api/sync/manifest" && r.Method == http.MethodGet
		if strings.HasPrefix(path, "/api/sync/projects/") && validID.MatchString(strings.TrimPrefix(path, "/api/sync/projects/")) {
			allowed = r.Method == http.MethodGet || r.Method == http.MethodPut
		}
		if !allowed || r.URL.RawQuery != "" || r.URL.RawPath != "" {
			reject()
			return
		}
		if r.Method == http.MethodPut && r.Header.Get("Content-Type") != "application/json" {
			reject()
			return
		}
		mux.ServeHTTP(w, r)
	})
}
