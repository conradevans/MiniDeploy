package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	testAccessIssuer   = "https://test.cloudflareaccess.com"
	testAccessAudience = "minideploy-test-audience"
	testAdminEmail     = "admin@example.com"
)

type accessTestToken struct {
	issuer    string
	audience  string
	email     string
	expiresAt time.Time
	notBefore time.Time
}

func TestCloudflareAccessValidator(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	otherPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate second RSA key: %v", err)
	}

	validator := newCloudflareAccessValidatorWithKeySet(
		testAccessIssuer,
		testAccessAudience,
		testAdminEmail,
		&oidc.StaticKeySet{
			PublicKeys: []crypto.PublicKey{
				&privateKey.PublicKey,
			},
		},
	)

	now := time.Now().UTC()
	valid := accessTestToken{
		issuer:    testAccessIssuer,
		audience:  testAccessAudience,
		email:     testAdminEmail,
		expiresAt: now.Add(time.Hour),
		notBefore: now.Add(-time.Minute),
	}

	tests := []struct {
		name       string
		token      string
		claims     accessTestToken
		signingKey *rsa.PrivateKey
		wantStatus int
	}{
		{
			name:       "valid assertion",
			claims:     valid,
			signingKey: privateKey,
			wantStatus: http.StatusOK,
		},
		{
			name:       "malformed assertion",
			token:      "not-a-jwt",
			wantStatus: http.StatusForbidden,
		},
		{
			name: "expired assertion",
			claims: accessTestToken{
				issuer:    valid.issuer,
				audience:  valid.audience,
				email:     valid.email,
				expiresAt: now.Add(-time.Hour),
				notBefore: valid.notBefore,
			},
			signingKey: privateKey,
			wantStatus: http.StatusForbidden,
		},
		{
			name: "not-yet-valid assertion",
			claims: accessTestToken{
				issuer:    valid.issuer,
				audience:  valid.audience,
				email:     valid.email,
				expiresAt: valid.expiresAt,
				notBefore: now.Add(10 * time.Minute),
			},
			signingKey: privateKey,
			wantStatus: http.StatusForbidden,
		},
		{
			name: "wrong audience",
			claims: accessTestToken{
				issuer:    valid.issuer,
				audience:  "another-audience",
				email:     valid.email,
				expiresAt: valid.expiresAt,
				notBefore: valid.notBefore,
			},
			signingKey: privateKey,
			wantStatus: http.StatusForbidden,
		},
		{
			name: "wrong issuer",
			claims: accessTestToken{
				issuer:    "https://other.cloudflareaccess.com",
				audience:  valid.audience,
				email:     valid.email,
				expiresAt: valid.expiresAt,
				notBefore: valid.notBefore,
			},
			signingKey: privateKey,
			wantStatus: http.StatusForbidden,
		},
		{
			name: "wrong email",
			claims: accessTestToken{
				issuer:    valid.issuer,
				audience:  valid.audience,
				email:     "someone@example.com",
				expiresAt: valid.expiresAt,
				notBefore: valid.notBefore,
			},
			signingKey: privateKey,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "wrong signature",
			claims:     valid,
			signingKey: otherPrivateKey,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawToken := tt.token
			if tt.signingKey != nil {
				rawToken = signAccessTestToken(
					t,
					tt.signingKey,
					tt.claims,
				)
			}

			req := httptest.NewRequest(
				http.MethodGet,
				"https://minideploy.reactorlab.dev/api/admin/session",
				nil,
			)
			req.Header.Set(accessJWTHeader, rawToken)

			recorder := httptest.NewRecorder()
			publicRoutes(validator).ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d; want %d; body=%s",
					recorder.Code,
					tt.wantStatus,
					recorder.Body.String(),
				)
			}

			if tt.wantStatus == http.StatusOK &&
				recorder.Body.String() !=
					`{"role":"admin","email":"admin@example.com"}`+"\n" {
				t.Fatalf(
					"unexpected session response: %s",
					recorder.Body.String(),
				)
			}
		})
	}
}

func signAccessTestToken(
	t *testing.T,
	privateKey *rsa.PrivateKey,
	claims accessTestToken,
) string {
	t.Helper()

	signer, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: jose.RS256,
			Key:       privateKey,
		},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		t.Fatalf("create JWT signer: %v", err)
	}

	standardClaims := jwt.Claims{
		Issuer:    claims.issuer,
		Audience:  jwt.Audience{claims.audience},
		Expiry:    jwt.NewNumericDate(claims.expiresAt),
		NotBefore: jwt.NewNumericDate(claims.notBefore),
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
	}

	privateClaims := struct {
		Email string `json:"email"`
	}{
		Email: claims.email,
	}

	rawToken, err := jwt.Signed(signer).
		Claims(standardClaims).
		Claims(privateClaims).
		Serialize()
	if err != nil {
		t.Fatalf("sign Access test token: %v", err)
	}

	return rawToken
}

func TestPublicAdminRequiresAccessAssertion(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodGet,
		"https://minideploy.reactorlab.dev/api/admin/session",
		nil,
	)
	recorder := httptest.NewRecorder()

	publicRoutes(rejectingAccessValidator{}).
		ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status = %d; want %d",
			recorder.Code,
			http.StatusUnauthorized,
		)
	}
}

func TestPublicAdminDoesNotTrustPlainIdentityHeaders(
	t *testing.T,
) {
	req := httptest.NewRequest(
		http.MethodGet,
		"https://minideploy.reactorlab.dev/api/admin/session",
		nil,
	)
	req.Header.Set(
		"Cf-Access-Authenticated-User-Email",
		testAdminEmail,
	)
	recorder := httptest.NewRecorder()

	publicRoutes(stubAccessValidator{
		identity: AccessIdentity{
			Email: testAdminEmail,
		},
	}).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"plain identity header status = %d; want %d",
			recorder.Code,
			http.StatusUnauthorized,
		)
	}
}

func TestEveryPublicAdminRouteRequiresAccess(
	t *testing.T,
) {
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/admin"},
		{http.MethodGet, "/admin/"},
		{http.MethodGet, "/admin/future-route"},
		{http.MethodGet, "/api/admin/session"},
		{http.MethodPost, "/api/admin/deploy"},
		{http.MethodGet, "/api/admin/deployments"},
		{http.MethodGet, "/api/admin/deployments/app/logs"},
		{http.MethodGet, "/api/admin/deployments/app/deploy-logs"},
		{http.MethodGet, "/api/admin/deployments/app/history"},
		{http.MethodPost, "/api/admin/deployments/app/restart"},
		{http.MethodPost, "/api/admin/deployments/app/redeploy"},
		{http.MethodPost, "/api/admin/deployments/app/rollback"},
		{http.MethodDelete, "/api/admin/deployments/app"},
		{http.MethodGet, "/api/admin/future-route"},
	}

	handler := publicRoutes(stubAccessValidator{})

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(
				tt.method,
				"https://minideploy.reactorlab.dev"+tt.path,
				nil,
			)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf(
					"status = %d; want %d",
					recorder.Code,
					http.StatusUnauthorized,
				)
			}
		})
	}
}

func TestMissingAccessConfigurationFailsClosed(t *testing.T) {
	_, err := newCloudflareAccessValidator(AccessConfig{})
	if err == nil {
		t.Fatal("expected missing Access configuration to fail")
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"https://minideploy.reactorlab.dev/api/admin/session",
		nil,
	)
	req.Header.Set(accessJWTHeader, "forged")
	recorder := httptest.NewRecorder()

	publicRoutes(nil).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf(
			"status = %d; want %d",
			recorder.Code,
			http.StatusForbidden,
		)
	}
}

func TestAccessConfigFromEnvironment(t *testing.T) {
	t.Setenv(
		"MINIDEPLOY_ACCESS_TEAM_DOMAIN",
		"https://example.cloudflareaccess.com",
	)
	t.Setenv(
		"MINIDEPLOY_ACCESS_AUDIENCE",
		"audience",
	)
	t.Setenv(
		"MINIDEPLOY_ACCESS_ADMIN_EMAIL",
		"Admin@Example.com",
	)

	config := accessConfigFromEnvironment()

	if config.TeamDomain !=
		"https://example.cloudflareaccess.com" ||
		config.Audience != "audience" ||
		config.AdminEmail != "Admin@Example.com" {

		t.Fatalf("unexpected Access configuration: %#v", config)
	}
}

func TestAccessValidatorInterfaceCanBeInjected(t *testing.T) {
	validator := AccessTokenValidator(
		stubAccessValidator{
			identity: AccessIdentity{
				Email: testAdminEmail,
			},
		},
	)

	identity, err := validator.Validate(
		context.Background(),
		"test-token",
	)
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}

	if identity.Email != testAdminEmail {
		t.Fatalf(
			"email = %q; want %q",
			identity.Email,
			testAdminEmail,
		)
	}
}

type stubAccessValidator struct {
	identity AccessIdentity
	err      error
}

func (s stubAccessValidator) Validate(
	context.Context,
	string,
) (AccessIdentity, error) {
	return s.identity, s.err
}
