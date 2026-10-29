package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSyncPortExposesOnlyAuthenticatedTLSProtocol(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: store, Remote: true, AuthKey: syncTestKey, Assets: fstest.MapFS{"index.html": {Data: []byte("PRIVATE EDITOR")}}}
	handler := s.SyncHandler()
	for _, path := range []string{"/", "/index.html", "/assets/app.js", "/api/server-info", "/api/auth/login", "/api/projects", "/api/config", "/api/agent", "/api/sync/settings", "/api/sync/run", "/api/backups", "/api/backups/settings", "/api/sync/catalog", "/api/sync/manifest", "/api/sync/projects/invalid", "/api/sync//manifest"} {
		for _, auth := range []string{"", "Bearer wrong-secret", "Bearer " + syncTestKey} {
			r := httptest.NewRequest("GET", "https://server.example"+path, nil)
			r.Header.Set("Authorization", auth)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if path == "/api/sync/manifest" && auth == "Bearer "+syncTestKey {
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
			} else if w.Code != 404 || w.Body.Len() != 0 || w.Header().Get("Location") != "" {
				t.Fatalf("port exposed response for %s: %d %q", path, w.Code, w.Body.String())
			}
		}
	}
	for _, test := range []struct{ url, origin, method string }{
		{"http://server.example/api/sync/manifest", "", "GET"},
		{"https://server.example/api/sync/manifest", "https://server.example", "GET"},
		{"https://server.example/api/sync/manifest", "", "OPTIONS"},
		{"https://server.example/api/sync/manifest?x=1", "", "GET"},
	} {
		r := httptest.NewRequest(test.method, test.url, nil)
		r.Header.Set("Authorization", "Bearer "+syncTestKey)
		r.Header.Set("Origin", test.origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 404 || w.Body.Len() != 0 {
			t.Fatal("non-protocol request accepted", test)
		}
	}
}

func TestSyncTLSCertificatePinAndDowngrade(t *testing.T) {
	store, _ := NewStore(t.TempDir())
	server := httptest.NewTLSServer((&Server{Store: store, Remote: true, AuthKey: syncTestKey}).SyncHandler())
	defer server.Close()
	digest := sha256.Sum256(server.Certificate().Raw)
	cfg := SyncSettings{ServerURL: server.URL, Token: syncTestKey, TLSFingerprint: hex.EncodeToString(digest[:]), IntervalMinutes: 5}
	var manifest SyncManifest
	if err := syncRemote(context.Background(), cfg, "GET", "/manifest", nil, &manifest); err != nil {
		t.Fatal(err)
	}
	cfg.TLSFingerprint = strings.Repeat("0", 64)
	if err := syncRemote(context.Background(), cfg, "GET", "/manifest", nil, &manifest); err == nil {
		t.Fatal("wrong pinned certificate accepted")
	}
	cfg.TLSFingerprint = ""
	if err := syncRemote(context.Background(), cfg, "GET", "/manifest", nil, &manifest); err == nil {
		t.Fatal("untrusted certificate accepted without pin")
	}
	for _, url := range []string{"http://127.0.0.1:48120", "http://example.com", "https://user:pw@example.com", "https://example.com/path"} {
		cfg.ServerURL = url
		if _, err := store.SaveSyncSettings(cfg); err == nil {
			t.Fatal("unsafe URL accepted", url)
		}
	}
	cfg.ServerURL = server.URL
	cfg.TLSFingerprint = "invalid"
	if _, err := store.SaveSyncSettings(cfg); err == nil {
		t.Fatal("invalid pin accepted")
	}
	// Old plaintext configuration must not leak the key before validation.
	hits := 0
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer plain.Close()
	cfg.ServerURL = plain.URL
	cfg.TLSFingerprint = ""
	if err := syncRemote(context.Background(), cfg, "GET", "/manifest", nil, &manifest); err == nil || hits != 0 {
		t.Fatal("plaintext request sent")
	}
	// Never follow redirects with credentials, including HTTPS-to-HTTP redirects.
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	digest = sha256.Sum256(redirect.Certificate().Raw)
	cfg.ServerURL = redirect.URL
	cfg.TLSFingerprint = hex.EncodeToString(digest[:])
	if err := syncRemote(context.Background(), cfg, "GET", "/manifest", nil, &manifest); err == nil || hits != 0 {
		t.Fatal("redirect followed")
	}
}
