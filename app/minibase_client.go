package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultMiniBaseURL       = "http://127.0.0.1:9100"
	defaultMiniBaseTokenPath = "/srv/minibase/secrets/minideploy-integration-token"
	miniBaseResponseLimit    = 64 << 10
	miniBaseRequestTimeout   = 15 * time.Second
	miniBaseBindingPrimary   = "primary"
	reactorLabDataNetwork    = "reactorlab-data"
)

var (
	miniBaseDatabaseIDPattern               = regexp.MustCompile(`^database_[0-9a-f]{32}$`)
	miniBaseAttachmentIDPattern             = regexp.MustCompile(`^attachment_[0-9a-f]{32}$`)
	miniBaseInternalDBPattern               = regexp.MustCompile(`^mb_db_[0-9a-f]{32}$`)
	miniBaseRolePattern                     = regexp.MustCompile(`^mb_role_[0-9a-f]{32}$`)
	miniBaseClient              miniBaseAPI = defaultMiniBaseHTTPClient()
)

type miniBaseDatabase struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
	Attached    bool   `json:"attached"`
}

type miniBaseAttachment struct {
	ID           string `json:"id"`
	DatabaseID   string `json:"databaseId"`
	ConsumerType string `json:"consumerType"`
	ConsumerRef  string `json:"consumerRef"`
	BindingName  string `json:"bindingName"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type miniBaseBinding struct {
	DatabaseID    string `json:"databaseId"`
	Engine        string `json:"engine"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Database      string `json:"database"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	DockerNetwork string `json:"dockerNetwork"`
}

type miniBaseAPI interface {
	ListDatabases(context.Context) ([]miniBaseDatabase, error)
	CreateDatabase(context.Context, string) (miniBaseDatabase, error)
	CreateAttachment(context.Context, string, string) (miniBaseAttachment, error)
	DeleteAttachment(context.Context, string) error
	ResolveBinding(context.Context, string) (miniBaseBinding, error)
}

type miniBaseHTTPClient struct {
	baseURL   *url.URL
	tokenPath string
	client    *http.Client
}

type miniBaseHTTPError struct{ status int }

func (err miniBaseHTTPError) Error() string {
	return fmt.Sprintf("MiniBase request failed with HTTP %d", err.status)
}

func defaultMiniBaseHTTPClient() miniBaseAPI {
	client, err := newMiniBaseHTTPClient(defaultMiniBaseURL, defaultMiniBaseTokenPath, &http.Client{Timeout: miniBaseRequestTimeout})
	if err != nil {
		panic(err)
	}
	return client
}

func newMiniBaseHTTPClient(rawURL, tokenPath string, client *http.Client) (*miniBaseHTTPClient, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, fmt.Errorf("invalid MiniBase URL")
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil || port == "" {
		return nil, fmt.Errorf("invalid MiniBase URL")
	}
	portNumber, portErr := strconv.Atoi(port)
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() || portErr != nil || portNumber < 1 || portNumber > 65535 {
		return nil, fmt.Errorf("MiniBase URL must use a loopback IP")
	}
	if strings.TrimSpace(tokenPath) == "" {
		return nil, fmt.Errorf("MiniBase token path must not be empty")
	}
	if client == nil {
		client = &http.Client{Timeout: miniBaseRequestTimeout}
	}
	return &miniBaseHTTPClient{baseURL: parsed, tokenPath: tokenPath, client: client}, nil
}

func loadMiniBaseToken(tokenPath string) ([]byte, error) {
	info, err := os.Lstat(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("MiniBase integration authentication unavailable")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("MiniBase integration authentication unavailable")
	}
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("MiniBase integration authentication unavailable")
	}
	token = []byte(strings.TrimSpace(string(token)))
	if len(token) < 32 || strings.ContainsAny(string(token), " \t\r\n") {
		return nil, fmt.Errorf("MiniBase integration authentication unavailable")
	}
	return token, nil
}

func (client *miniBaseHTTPClient) endpoint(endpointPath string) string {
	resolved := *client.baseURL
	resolved.Path = path.Clean("/" + strings.TrimPrefix(endpointPath, "/"))
	return resolved.String()
}

func (client *miniBaseHTTPClient) doJSON(ctx context.Context, method, endpointPath string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode MiniBase request")
		}
		body = bytes.NewReader(encoded)
	}
	requestContext, cancel := context.WithTimeout(ctx, miniBaseRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, method, client.endpoint(endpointPath), body)
	if err != nil {
		return fmt.Errorf("prepare MiniBase request")
	}
	token, err := loadMiniBaseToken(client.tokenPath)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+string(token))
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("MiniBase is unavailable")
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, miniBaseResponseLimit+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read MiniBase response")
	}
	if len(content) > miniBaseResponseLimit {
		return fmt.Errorf("MiniBase response exceeded the safety limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return miniBaseHTTPError{status: response.StatusCode}
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("MiniBase returned an invalid response")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("MiniBase returned an invalid response")
	}
	return nil
}

func (client *miniBaseHTTPClient) ListDatabases(ctx context.Context) ([]miniBaseDatabase, error) {
	var databases []miniBaseDatabase
	if err := client.doJSON(ctx, http.MethodGet, "/api/v1/integrations/minideploy/databases", nil, &databases); err != nil {
		return nil, err
	}
	if databases == nil {
		databases = []miniBaseDatabase{}
	}
	for _, database := range databases {
		if !validMiniBaseDatabase(database) {
			return nil, fmt.Errorf("MiniBase returned an invalid response")
		}
	}
	return databases, nil
}

func (client *miniBaseHTTPClient) CreateDatabase(ctx context.Context, displayName string) (miniBaseDatabase, error) {
	var database miniBaseDatabase
	if err := client.doJSON(ctx, http.MethodPost, "/api/v1/integrations/minideploy/databases", map[string]string{"displayName": displayName}, &database); err != nil {
		return miniBaseDatabase{}, err
	}
	if !validMiniBaseDatabase(database) {
		return miniBaseDatabase{}, fmt.Errorf("MiniBase returned an invalid response")
	}
	return database, nil
}

func (client *miniBaseHTTPClient) CreateAttachment(ctx context.Context, databaseID, consumerRef string) (miniBaseAttachment, error) {
	var attachment miniBaseAttachment
	input := map[string]string{"databaseId": databaseID, "consumerRef": consumerRef, "bindingName": miniBaseBindingPrimary}
	if err := client.doJSON(ctx, http.MethodPost, "/api/v1/integrations/minideploy/attachments", input, &attachment); err != nil {
		return miniBaseAttachment{}, err
	}
	if !miniBaseAttachmentIDPattern.MatchString(attachment.ID) || attachment.DatabaseID != databaseID || attachment.ConsumerType != "minideploy" || attachment.ConsumerRef != consumerRef || attachment.BindingName != miniBaseBindingPrimary {
		return miniBaseAttachment{}, fmt.Errorf("MiniBase returned an invalid response")
	}
	return attachment, nil
}

func (client *miniBaseHTTPClient) DeleteAttachment(ctx context.Context, attachmentID string) error {
	if !miniBaseAttachmentIDPattern.MatchString(attachmentID) {
		return fmt.Errorf("invalid MiniBase attachment")
	}
	err := client.doJSON(ctx, http.MethodDelete, "/api/v1/integrations/minideploy/attachments/"+url.PathEscape(attachmentID), nil, nil)
	var httpError miniBaseHTTPError
	if errors.As(err, &httpError) && httpError.status == http.StatusNotFound {
		return nil
	}
	return err
}

func (client *miniBaseHTTPClient) ResolveBinding(ctx context.Context, attachmentID string) (miniBaseBinding, error) {
	if !miniBaseAttachmentIDPattern.MatchString(attachmentID) {
		return miniBaseBinding{}, fmt.Errorf("invalid MiniBase attachment")
	}
	var binding miniBaseBinding
	endpoint := "/api/v1/integrations/minideploy/attachments/" + url.PathEscape(attachmentID) + "/binding"
	if err := client.doJSON(ctx, http.MethodGet, endpoint, nil, &binding); err != nil {
		return miniBaseBinding{}, err
	}
	if !validMiniBaseBinding(binding) {
		return miniBaseBinding{}, fmt.Errorf("MiniBase returned an invalid binding")
	}
	return binding, nil
}

func validMiniBaseDatabase(database miniBaseDatabase) bool {
	return miniBaseDatabaseIDPattern.MatchString(database.ID) &&
		strings.TrimSpace(database.DisplayName) == database.DisplayName &&
		database.DisplayName != "" && utf8.RuneCountInString(database.DisplayName) <= 200 &&
		database.Status == "ready"
}

func validMiniBaseBinding(binding miniBaseBinding) bool {
	return miniBaseDatabaseIDPattern.MatchString(binding.DatabaseID) &&
		binding.Engine == "postgresql" && binding.Host == "minibase-postgres" &&
		binding.Port == 5432 && miniBaseInternalDBPattern.MatchString(binding.Database) &&
		miniBaseRolePattern.MatchString(binding.Username) && binding.Password != "" &&
		!strings.ContainsAny(binding.Password, "\r\n\x00") &&
		binding.DockerNetwork == reactorLabDataNetwork
}
