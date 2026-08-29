package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	proxyRoutesPath = "/srv/minideploy/caddy/apps.caddy"
	caddyConfigPath = "/etc/caddy/Caddyfile"
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

	var config strings.Builder

	for _, record := range records {
		if record.App == "" || record.Port <= 0 {
			continue
		}

		fmt.Fprintf(
			&config,
			"redir /%s /%s/\n\n",
			record.App,
			record.App,
		)

		fmt.Fprintf(
			&config,
			"handle_path /%s/* {\n"+
				"\treverse_proxy 127.0.0.1:%d\n"+
				"}\n\n",
			record.App,
			record.Port,
		)
	}

	if err := os.MkdirAll(
		filepath.Dir(proxyRoutesPath),
		0755,
	); err != nil {
		return fmt.Errorf(
			"create proxy directory: %w",
			err,
		)
	}

	tempPath := proxyRoutesPath + ".tmp"

	if err := os.WriteFile(
		tempPath,
		[]byte(config.String()),
		0644,
	); err != nil {
		return fmt.Errorf(
			"write proxy configuration: %w",
			err,
		)
	}

	if err := os.Rename(
		tempPath,
		proxyRoutesPath,
	); err != nil {
		return fmt.Errorf(
			"replace proxy configuration: %w",
			err,
		)
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
