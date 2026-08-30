package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityMiddlewareAllowsSameOriginManagementPost(
	t *testing.T,
) {
	next := http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		w.WriteHeader(http.StatusNoContent)
	})

	handler := securityMiddleware(next)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://localhost:9000/deployments/test/redeploy",
		nil,
	)
	req.Header.Set(
		"Origin",
		"http://localhost:9000",
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusNoContent,
			recorder.Code,
		)
	}
}

func TestSecurityMiddlewareRejectsCrossOriginManagementPost(
	t *testing.T,
) {
	next := http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		w.WriteHeader(http.StatusNoContent)
	})

	handler := securityMiddleware(next)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://localhost:9000/deployments/test/redeploy",
		nil,
	)
	req.Header.Set(
		"Origin",
		"https://evil.example",
	)
	req.Header.Set(
		"Sec-Fetch-Site",
		"cross-site",
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusForbidden,
			recorder.Code,
		)
	}
}

func TestSecurityMiddlewareAllowsWebhookPost(
	t *testing.T,
) {
	next := http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		w.WriteHeader(http.StatusNoContent)
	})

	handler := securityMiddleware(next)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://localhost:9000/webhooks/github",
		nil,
	)
	req.Header.Set(
		"Origin",
		"https://github.com",
	)
	req.Header.Set(
		"Sec-Fetch-Site",
		"cross-site",
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf(
			"expected webhook to bypass management CSRF check, got %d",
			recorder.Code,
		)
	}
}

func TestSecurityHeaders(t *testing.T) {
	next := http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		w.WriteHeader(http.StatusOK)
	})

	handler := securityMiddleware(next)

	req := httptest.NewRequest(
		http.MethodGet,
		"http://localhost:9000/",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	required := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Permissions-Policy",
		"Content-Security-Policy",
		"Cache-Control",
	}

	for _, header := range required {
		if recorder.Header().Get(header) == "" {
			t.Fatalf(
				"expected security header %s",
				header,
			)
		}
	}
}
