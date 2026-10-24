package workspace

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) serverAuthenticated(r *http.Request) bool {
	if len(s.AuthKey) < 16 {
		return false
	}
	if token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.AuthKey)) == 1 {
		return true
	}
	cookie, err := r.Cookie("yantai_session")
	if err != nil {
		return false
	}
	expiry, signature, ok := strings.Cut(cookie.Value, ".")
	if !ok {
		return false
	}
	until, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || until <= time.Now().Unix() {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(s.sessionSignature(expiry)))
}
func (s *Server) sessionSignature(expiry string) string {
	mac := hmac.New(sha256.New, []byte(s.AuthKey))
	mac.Write([]byte("yantai-session-v1:" + expiry))
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Server) registerServerAuth(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/server-info", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"remote": s.Remote, "authenticated": !s.Remote || s.serverAuthenticated(r)})
	})
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Token string `json:"token"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		if !s.Remote || len(s.AuthKey) < 16 || subtle.ConstantTimeCompare([]byte(in.Token), []byte(s.AuthKey)) != 1 {
			reply(w, 401, map[string]string{"error": "服务端访问密钥不正确"})
			return
		}
		expiry := strconv.FormatInt(time.Now().Add(12*time.Hour).Unix(), 10)
		http.SetCookie(w, &http.Cookie{Name: "yantai_session", Value: expiry + "." + s.sessionSignature(expiry), Path: "/", HttpOnly: true, Secure: s.SecureCookie || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 12 * 3600})
		reply(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "yantai_session", Value: "", Path: "/", HttpOnly: true, Secure: s.SecureCookie || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		reply(w, 200, map[string]bool{"ok": true})
	})
}
