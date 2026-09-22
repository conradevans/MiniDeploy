package main

import (
	"fmt"
	"net/url"
	"strings"
)

func validateRepositoryURL(repositoryURL string) error {
	candidate := strings.TrimSpace(repositoryURL)
	parsed, err := url.Parse(candidate)
	if err != nil {
		if hasHTTPRepositoryScheme(candidate) {
			return fmt.Errorf("invalid HTTP repository URL")
		}
		return nil
	}

	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			return fmt.Errorf("repository URL must not contain credentials")
		}
	}

	if !strings.EqualFold(parsed.Scheme, "http") &&
		!strings.EqualFold(parsed.Scheme, "https") {

		return nil
	}

	if parsed.Host == "" || parsed.Opaque != "" {
		return fmt.Errorf("invalid HTTP repository URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("repository URL must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || strings.Contains(candidate, "#") {

		return fmt.Errorf(
			"repository URL must not contain a query string or fragment",
		)
	}

	return nil
}

func hasHTTPRepositoryScheme(repositoryURL string) bool {
	separator := strings.IndexByte(repositoryURL, ':')
	if separator < 0 {
		return false
	}

	scheme := repositoryURL[:separator]
	return strings.EqualFold(scheme, "http") ||
		strings.EqualFold(scheme, "https")
}
