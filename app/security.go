package main

import (
	"net/http"
	"net/url"
	"strings"
)

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		setSecurityHeaders(w)

		if isUnsafeMethod(r.Method) &&
			r.URL.Path != "/webhooks/github" &&
			!allowedManagementRequest(r) {

			http.Error(
				w,
				"cross-origin management request rejected",
				http.StatusForbidden,
			)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set(
		"X-Content-Type-Options",
		"nosniff",
	)

	w.Header().Set(
		"X-Frame-Options",
		"DENY",
	)

	w.Header().Set(
		"Referrer-Policy",
		"no-referrer",
	)

	w.Header().Set(
		"Permissions-Policy",
		"camera=(), microphone=(), geolocation=()",
	)

	w.Header().Set(
		"Content-Security-Policy",
		"default-src 'self'; "+
			"script-src 'self' 'unsafe-inline'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"connect-src 'self'; "+
			"img-src 'self' data:; "+
			"object-src 'none'; "+
			"base-uri 'self'; "+
			"frame-ancestors 'none'",
	)

	w.Header().Set(
		"Cache-Control",
		"no-store",
	)
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete:
		return true
	default:
		return false
	}
}

func allowedManagementRequest(
	r *http.Request,
) bool {
	if strings.EqualFold(
		r.Header.Get("Sec-Fetch-Site"),
		"cross-site",
	) {
		return false
	}

	origin := strings.TrimSpace(
		r.Header.Get("Origin"),
	)

	// Command-line clients such as curl do not normally send
	// Origin. The management server is loopback-only, so these
	// requests are still allowed.
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	if parsed.Scheme != "http" {
		return false
	}

	if parsed.Port() != "9000" {
		return false
	}

	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
