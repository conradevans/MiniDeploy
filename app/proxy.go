package main

import (
	"errors"
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
	publicSiteHostname    = "minideploy.reactorlab.dev"
)

var ErrReservedPublicHostname = errors.New(
	"public hostname is reserved",
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
		record = normalizeDeploymentRecord(record)
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

		hostname := publicHostnameForApp(record.App)
		if record.Strategy == deploymentStrategyFullstackViteNode {
			localRoutes, publicRoutes, err :=
				fullstackProxyRouteFragments(
					record,
					publicIndex,
					hostname,
				)
			if err != nil {
				return fmt.Errorf(
					"prepare full-stack proxy for %s: %w",
					record.App,
					err,
				)
			}
			localConfig.WriteString(localRoutes)
			publicConfig.WriteString(publicRoutes)

			publicIndex++
			continue
		}

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

func fullstackProxyServices(
	record DeploymentRecord,
) (DeploymentServiceRecord, DeploymentServiceRecord, error) {
	if err := validateFullstackServiceMetadata(record.Services); err != nil {
		return DeploymentServiceRecord{}, DeploymentServiceRecord{}, err
	}
	frontend, _ := deploymentServiceByName(record, fullstackFrontendService)
	backend, _ := deploymentServiceByName(record, fullstackBackendService)
	if frontend.Port <= 0 || backend.Port <= 0 {
		return DeploymentServiceRecord{}, DeploymentServiceRecord{},
			fmt.Errorf("full-stack service ports are unavailable")
	}
	return frontend, backend, nil
}

func fullstackProxyRouteFragments(
	record DeploymentRecord,
	index int,
	hostname string,
) (string, string, error) {
	frontend, backend, err := fullstackProxyServices(record)
	if err != nil {
		return "", "", err
	}

	localRoutes := fmt.Sprintf(
		"handle_path /%s/* {\n"+
			"\t@local_api_%d path /api /api/*\n"+
			"\thandle @local_api_%d {\n"+
			"\t\treverse_proxy 127.0.0.1:%d\n"+
			"\t}\n"+
			"\thandle {\n"+
			"\t\treverse_proxy 127.0.0.1:%d\n"+
			"\t}\n"+
			"}\n\n",
		record.App,
		index,
		index,
		backend.Port,
		frontend.Port,
	)

	if hostname == "" {
		return localRoutes, "", nil
	}
	publicRoutes := fmt.Sprintf(
		"@public_api_%d {\n"+
			"\thost %s\n"+
			"\tpath /api /api/*\n"+
			"}\n"+
			"handle @public_api_%d {\n"+
			"\treverse_proxy 127.0.0.1:%d\n"+
			"}\n\n"+
			"@public_app_%d host %s\n"+
			"handle @public_app_%d {\n"+
			"\treverse_proxy 127.0.0.1:%d\n"+
			"}\n\n",
		index,
		hostname,
		index,
		backend.Port,
		index,
		hostname,
		index,
		frontend.Port,
	)
	return localRoutes, publicRoutes, nil
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
	result := publicHostnameLabel(app)
	if result == "" ||
		result+"."+publicDomain == publicSiteHostname {

		return ""
	}

	return result + "." + publicDomain
}

func publicHostnameLabel(app string) string {
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

	return result
}

func isReservedPublicApp(app string) bool {
	label := publicHostnameLabel(app)

	return label != "" &&
		label+"."+publicDomain == publicSiteHostname
}
