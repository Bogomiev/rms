package handler

import (
	"net/http"
	"time"
)

func (h *Handler) setTokenCookies(w http.ResponseWriter, access, refresh string) {
	setCookie(w, "access_token", access, http.SameSiteLaxMode, h.accessTTL)
	setCookie(w, "refresh_token", refresh, http.SameSiteStrictMode, h.refreshTTL)
}
func setCookie(w http.ResponseWriter, name, value string, sameSite http.SameSite, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: true, SameSite: sameSite, MaxAge: int(ttl / time.Second), Expires: time.Now().Add(ttl)})
}
func clearTokenCookies(w http.ResponseWriter) {
	for name, mode := range map[string]http.SameSite{"access_token": http.SameSiteLaxMode, "refresh_token": http.SameSiteStrictMode} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, Secure: true, SameSite: mode, MaxAge: -1, Expires: time.Unix(1, 0)})
	}
}
