package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"yantai/internal/workspace"
)

func TestIntranetWebAddressAndPeer(t *testing.T) {
	for _, address := range []string{"172.16.1.200:48121", "192.168.1.10:48121", "10.0.0.1:48121", "127.0.0.1:48121", "[fd00::1]:48121", "localhost:48121"} {
		if !privateWebAddress(address) {
			t.Fatal("private bind denied", address)
		}
	}
	for _, address := range []string{"0.0.0.0:48121", ":48121", "[::]:48121", "8.8.8.8:48121", "server.example:48121"} {
		if privateWebAddress(address) {
			t.Fatal("unsafe bind accepted", address)
		}
	}
	store, err := workspace.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := "intranet-web-fixture-secret"
	s := &workspace.Server{Store: store, Remote: true, AuthKey: key, Assets: fstest.MapFS{"index.html": {Data: []byte("LAN WORKSPACE")}}}
	handler := intranetWebHandler(s.Handler())
	request := func(peer, path, method, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://172.16.1.200:48121"+path, strings.NewReader(body))
		r.RemoteAddr = peer
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-For", "192.168.1.1")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, peer := range []string{"8.8.8.8:1234", "[2001:4860:4860::8888]:1234", "invalid"} {
		w := request(peer, "/api/server-info", "GET", "", nil)
		if w.Code != 404 || w.Body.Len() != 0 {
			t.Fatal("public peer accepted", peer)
		}
	}
	w := request("172.16.1.20:1234", "/index.html", "GET", "", nil)
	// FileServer may redirect /index.html to /; neither route goes to the public sync listener.
	if w.Code != 200 && w.Code != 301 {
		t.Fatal("LAN page unavailable", w.Code)
	}
	w = request("172.16.1.20:1234", "/api/projects", "GET", "", nil)
	if w.Code != 401 {
		t.Fatal("LAN auth missing", w.Code)
	}
	w = request("172.16.1.20:1234", "/api/auth/login", "POST", `{"token":"`+key+`"}`, nil)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal("LAN login failed", w.Code)
	}
	w = request("172.16.1.20:1234", "/api/projects", "GET", "", w.Result().Cookies()[0])
	if w.Code != 200 {
		t.Fatal("LAN authenticated API unavailable", w.Code)
	}
}
