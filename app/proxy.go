package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	proxyRoutesPath       = "/srv/minideploy/caddy/apps.caddy"
	publicProxyRoutesPath = "/srv/minideploy/caddy/public-apps.caddy"
	caddyConfigPath       = "/etc/caddy/Caddyfile"
	publicDomain          = "reactorlab.dev"
)

func syncProxyRoutes() error {
	records, err := store.List()
	if err != nil {
		return fmt.Errorf(
			"load deployments for proxy: %w",
			err,
		)
	}

	sort.Slice(
		records,
		func(i, j int) bool {
			return records[i].App < records[j].App
		},
	)

	var localConfig strings.Builder
	var publicConfig strings.Builder

	publicIndex := 0

	for _, record := range records {
		if record.App == "" || record.Port <= 0 {
			continue
		}

		// Existing local path-based routing:
		// https://mini-server.local/<app>/
		fmt.Fprintf(
			&localConfig,
			"redir /%s /%s/\n\n",
			record.App,
			record.App,
		)

		fmt.Fprintf(
			&localConfig,
			"handle_path /%s/* {\n"+
				"\treverse_proxy 127.0.0.1:%d\n"+
				"}\n\n",
			record.App,
			record.Port,
		)

		// Public hostname routing:
		// https://<app>.reactorlab.dev/
		hostname := publicHostnameForApp(record.App)
		if hostname == "" {
			continue
		}

		fmt.Fprintf(
			&publicConfig,
			"@public_app_%d host %s\n"+
				"handle @public_app_%d {\n"+
				"\treverse_proxy 127.0.0.1:%d\n"+
				"}\n\n",
			publicIndex,
			hostname,
			publicIndex,
			record.Port,
		)

		publicIndex++
	}

	if err := writeProxyRoutes(
		proxyRoutesPath,
		localConfig.String(),
	); err != nil {
		return err
	}

	if err := writeProxyRoutes(
		publicProxyRoutesPath,
		publicConfig.String(),
	); err != nil {
		return err
	}

	output, err := runCommand(
		"",
		"/usr/bin/caddy",
		"reload",
		"--config",
		caddyConfigPath,
		"--adapter",
		"caddyfile",
	)
	if err != nil {
		return fmt.Errorf(
			"reload Caddy: %w: %s",
			err,
			output,
		)
	}

	return nil
}

func writeProxyRoutes(
	path string,
	config string,
) error {
	if err := os.MkdirAll(
		filepath.Dir(path),
		0755,
	); err != nil {
		return fmt.Errorf(
			"create proxy directory: %w",
			err,
		)
	}

	tempPath := path + ".tmp"

	if err := os.WriteFile(
		tempPath,
		[]byte(config),
		0644,
	); err != nil {
		return fmt.Errorf(
			"write proxy configuration: %w",
			err,
		)
	}

	if err := os.Rename(
		tempPath,
		path,
	); err != nil {
		return fmt.Errorf(
			"replace proxy configuration: %w",
			err,
		)
	}

	return nil
}

func publicHostnameForApp(app string) string {
	app = strings.ToLower(
		strings.TrimSpace(app),
	)

	var label strings.Builder
	lastWasDash := false

	for _, r := range app {
		valid :=
			(r >= 'a' && r <= 'z') ||
				(r >= '0' && r <= '9') ||
				r == '-'

		if valid {
			label.WriteRune(r)
			lastWasDash = r == '-'
			continue
		}

		if label.Len() > 0 && !lastWasDash {
			label.WriteByte('-')
			lastWasDash = true
		}
	}

	result := strings.Trim(
		label.String(),
		"-",
	)

	if result == "" {
		return ""
	}

	if len(result) > 63 {
		result = strings.Trim(
			result[:63],
			"-",
		)
	}

	if result == "" {
		return ""
	}

	return result + "." + publicDomain
}
