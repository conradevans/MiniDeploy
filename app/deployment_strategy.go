package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	deploymentStrategyDockerfile        = "dockerfile"
	deploymentStrategyFullstackViteNode = "fullstack-vite-node"
	deploymentStrategyViteStatic        = "vite-static"
	deploymentStrategyNodeExpress       = "node-express"

	packageManagerNPM       = "npm"
	packageInstallModeCI    = "ci"
	packageInstallModeSetup = "install"
)

var ErrNoSupportedDeploymentStrategy = errors.New(
	"No supported deployment strategy detected. Add a Dockerfile or use a supported project type.",
)

type deploymentConfig struct {
	ContainerPort int
	HealthPath    string
}

type deploymentServiceBuildPlan struct {
	Name           string
	Path           string
	RepositoryPath string
	Build          deploymentBuildPlan
}

type deploymentBuildPlan struct {
	Strategy            string
	PackageManager      string
	PackageInstallMode  string
	ContainerPort       int
	HealthPath          string
	GeneratedDockerfile string
	Services            []deploymentServiceBuildPlan
	ReactorLabMigration bool
}

type deploymentStrategyDetector interface {
	Detect(
		repositoryPath string,
		requested deploymentConfig,
	) (deploymentBuildPlan, bool, error)
}

var deploymentStrategyDetectors = []deploymentStrategyDetector{
	dockerfileStrategyDetector{},
	fullstackViteNodeStrategyDetector{},
	viteStaticStrategyDetector{},
	nodeExpressStrategyDetector{},
}

type dockerfileStrategyDetector struct{}

func (dockerfileStrategyDetector) Detect(
	repositoryPath string,
	requested deploymentConfig,
) (deploymentBuildPlan, bool, error) {
	_, found, err := repositoryRegularFile(
		repositoryPath,
		"Dockerfile",
	)
	if err != nil {
		return deploymentBuildPlan{}, false, fmt.Errorf(
			"inspect Dockerfile: %w",
			err,
		)
	}
	if !found {
		return deploymentBuildPlan{}, false, nil
	}

	return deploymentBuildPlan{
		Strategy: deploymentStrategyDockerfile,
		ContainerPort: normalizedContainerPort(
			requested.ContainerPort,
		),
		HealthPath: normalizedHealthPath(
			requested.HealthPath,
		),
	}, true, nil
}

type viteStaticStrategyDetector struct{}

type nodePackageManifest struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	PackageManager  string            `json:"packageManager"`
	Workspaces      json.RawMessage   `json:"workspaces"`
}

type npmPackageLock struct {
	LockfileVersion int `json:"lockfileVersion"`
}

func (viteStaticStrategyDetector) Detect(
	repositoryPath string,
	_ deploymentConfig,
) (deploymentBuildPlan, bool, error) {
	manifest, found, err := readNodePackageManifest(
		repositoryPath,
	)
	if err != nil || !found {
		return deploymentBuildPlan{}, false, err
	}

	buildScript := strings.TrimSpace(
		manifest.Scripts["build"],
	)

	hasViteDependency := hasNodeDependency(
		manifest,
		"vite",
	)

	hasViteConfiguration, err := repositoryHasViteConfig(
		repositoryPath,
	)
	if err != nil {
		return deploymentBuildPlan{}, false, err
	}

	appendBuildSubcommand, supportedBuildScript := classifyViteBuildScript(
		buildScript,
	)
	if !supportedBuildScript ||
		(!hasViteDependency && !hasViteConfiguration) {

		return deploymentBuildPlan{}, false, nil
	}

	installMode, err := npmInstallMode(repositoryPath)
	if err != nil {
		return deploymentBuildPlan{}, false, err
	}

	return viteStaticBuildPlan(
		installMode,
		appendBuildSubcommand,
	), true, nil
}

// classifyViteBuildScript deliberately recognizes only Phase 1 build
// shapes where npm's appended CLI arguments reach Vite itself. The first
// result reports whether a bare final Vite command needs the build
// subcommand appended.
func classifyViteBuildScript(
	buildScript string,
) (bool, bool) {
	commands := strings.Split(buildScript, "&&")

	switch len(commands) {
	case 1:
		return classifyViteCommand(commands[0])
	case 2:
		if strings.Join(
			strings.Fields(commands[0]),
			" ",
		) != "tsc -b" {
			return false, false
		}

		return classifyViteCommand(commands[1])
	default:
		return false, false
	}
}

func classifyViteCommand(command string) (bool, bool) {
	fields := strings.Fields(command)

	if len(fields) == 1 && fields[0] == "vite" {
		return true, true
	}

	if len(fields) == 2 &&
		fields[0] == "vite" &&
		fields[1] == "build" {

		return false, true
	}

	return false, false
}

type nodeExpressStrategyDetector struct{}

var directNodeStartScriptPattern = regexp.MustCompile(
	`^node[ \t]+(\./)?[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*\.(js|cjs|mjs)$`,
)

var unsupportedNodeExpressDependencies = []string{
	"vite",
	"next",
	"@nestjs/core",
	"fastify",
	"koa",
	"typescript",
	"ts-node",
	"tsx",
}

func (nodeExpressStrategyDetector) Detect(
	repositoryPath string,
	requested deploymentConfig,
) (deploymentBuildPlan, bool, error) {
	manifest, found, err := readNodePackageManifest(
		repositoryPath,
	)
	if err != nil || !found {
		return deploymentBuildPlan{}, false, err
	}

	conventionalJavaScript, err :=
		repositoryIsConventionalJavaScriptExpress(
			repositoryPath,
			manifest,
		)
	if err != nil {
		return deploymentBuildPlan{}, false, err
	}

	if !conventionalJavaScript {
		return deploymentBuildPlan{}, false, nil
	}

	expressVersion, hasExpress := manifest.Dependencies["express"]
	if !hasExpress || strings.TrimSpace(expressVersion) == "" {
		return deploymentBuildPlan{}, false, nil
	}

	supportedEntrypoint, err := repositoryHasSupportedNodeEntrypoint(
		repositoryPath,
		manifest.Scripts["start"],
	)
	if err != nil {
		return deploymentBuildPlan{}, false, err
	}

	if !supportedEntrypoint {
		return deploymentBuildPlan{}, false, nil
	}

	usesNPM, err := repositoryUsesNPM(
		repositoryPath,
		manifest,
	)
	if err != nil {
		return deploymentBuildPlan{}, false, err
	}

	if !usesNPM {
		return deploymentBuildPlan{}, false, nil
	}

	installMode, err := npmInstallMode(repositoryPath)
	if err != nil {
		return deploymentBuildPlan{}, false, err
	}

	containerPort := requested.ContainerPort
	if containerPort == 0 {
		containerPort = defaultNodeContainerPort
	}

	healthPath := normalizedHealthPath(
		requested.HealthPath,
	)

	plan := nodeExpressBuildPlan(
		installMode,
		containerPort,
		healthPath,
	)
	plan.ReactorLabMigration = strings.TrimSpace(manifest.Scripts["reactorlab:migrate"]) != ""
	return plan, true, nil
}

// repositoryHasSupportedNodeEntrypoint deliberately accepts only the
// Phase 2 start-script shape "node <relative JavaScript file>". It is
// not a general shell-command parser.
func repositoryHasSupportedNodeEntrypoint(
	repositoryPath string,
	startScript string,
) (bool, error) {
	command := strings.TrimSpace(startScript)
	if !directNodeStartScriptPattern.MatchString(command) {
		return false, nil
	}

	entrypoint := strings.TrimLeft(
		strings.TrimPrefix(command, "node"),
		" \t",
	)
	if filepath.IsAbs(entrypoint) {
		return false, nil
	}

	entrypoint = strings.TrimPrefix(entrypoint, "./")
	for _, segment := range strings.Split(entrypoint, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false, nil
		}
	}

	cleanRoot, err := filepath.Abs(filepath.Clean(repositoryPath))
	if err != nil {
		return false, fmt.Errorf("resolve repository root: %w", err)
	}

	entrypointPath, err := strictChildPath(
		cleanRoot,
		filepath.Join(cleanRoot, filepath.FromSlash(entrypoint)),
	)
	if err != nil {
		return false, nil
	}

	resolvedRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return false, fmt.Errorf("resolve repository root symlinks: %w", err)
	}

	resolvedEntrypoint, err := filepath.EvalSymlinks(entrypointPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("resolve Node entrypoint symlinks: %w", err)
	}

	resolvedEntrypoint, err = strictChildPath(
		resolvedRoot,
		resolvedEntrypoint,
	)
	if err != nil {
		return false, nil
	}

	info, err := os.Stat(resolvedEntrypoint)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("inspect Node entrypoint: %w", err)
	}

	return info.Mode().IsRegular(), nil
}

func repositoryIsConventionalJavaScriptExpress(
	repositoryPath string,
	manifest nodePackageManifest,
) (bool, error) {
	workspaces := strings.TrimSpace(string(manifest.Workspaces))
	if workspaces != "" && workspaces != "null" {
		return false, nil
	}

	for _, name := range unsupportedNodeExpressDependencies {
		if hasNodeDependency(manifest, name) {
			return false, nil
		}
	}

	hasViteConfiguration, err := repositoryHasViteConfig(
		repositoryPath,
	)
	if err != nil {
		return false, err
	}

	if hasViteConfiguration {
		return false, nil
	}

	for _, field := range strings.Fields(
		strings.ToLower(manifest.Scripts["start"]),
	) {
		field = strings.Trim(field, "'\"")
		if field == "deno" {
			return false, nil
		}

		if strings.HasSuffix(field, ".ts") ||
			strings.HasSuffix(field, ".mts") ||
			strings.HasSuffix(field, ".cts") {

			return false, nil
		}
	}

	for _, name := range []string{
		"tsconfig.json",
		"deno.json",
		"deno.jsonc",
	} {
		info, err := os.Stat(filepath.Join(repositoryPath, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		if err != nil {
			return false, fmt.Errorf("inspect %s: %w", name, err)
		}

		if info.Mode().IsRegular() {
			return false, nil
		}
	}

	return true, nil
}

func repositoryUsesNPM(
	repositoryPath string,
	manifest nodePackageManifest,
) (bool, error) {
	packageManager := strings.TrimSpace(
		manifest.PackageManager,
	)
	if packageManager != "" &&
		packageManager != "npm" &&
		!strings.HasPrefix(packageManager, "npm@") {

		return false, nil
	}

	for _, name := range []string{
		"yarn.lock",
		"pnpm-lock.yaml",
		"bun.lock",
		"bun.lockb",
	} {
		_, err := os.Stat(filepath.Join(repositoryPath, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		if err != nil {
			return false, fmt.Errorf(
				"inspect %s: %w",
				name,
				err,
			)
		}

		return false, nil
	}

	return true, nil
}

func detectDeploymentStrategy(
	repositoryPath string,
	requested deploymentConfig,
) (deploymentBuildPlan, error) {
	for _, detector := range deploymentStrategyDetectors {
		plan, detected, err := detector.Detect(
			repositoryPath,
			requested,
		)

		if err != nil {
			return deploymentBuildPlan{}, err
		}

		if detected {
			return plan, nil
		}
	}

	return deploymentBuildPlan{},
		ErrNoSupportedDeploymentStrategy
}

func deploymentStrategyForRecord(
	record DeploymentRecord,
	repositoryPath string,
) (deploymentBuildPlan, error) {
	record = normalizeDeploymentRecord(record)

	switch record.Strategy {
	case deploymentStrategyDockerfile:
		plan, detected, err := (dockerfileStrategyDetector{}).Detect(
			repositoryPath,
			deploymentConfig{
				ContainerPort: record.ContainerPort,
				HealthPath:    record.HealthPath,
			},
		)
		if err != nil {
			return deploymentBuildPlan{}, err
		}

		if !detected {
			return deploymentBuildPlan{}, fmt.Errorf(
				"persisted Dockerfile strategy requires a Dockerfile",
			)
		}

		return plan, nil

	case deploymentStrategyFullstackViteNode:
		return persistedFullstackBuildPlan(record, repositoryPath)

	case deploymentStrategyViteStatic:
		if record.PackageManager != packageManagerNPM {
			return deploymentBuildPlan{}, fmt.Errorf(
				"unsupported persisted package manager %q",
				record.PackageManager,
			)
		}

		manifest, found, err := readNodePackageManifest(
			repositoryPath,
		)
		if err != nil {
			return deploymentBuildPlan{}, err
		} else if !found {
			return deploymentBuildPlan{}, fmt.Errorf(
				"persisted Vite strategy requires package.json",
			)
		}

		appendBuildSubcommand, supportedBuildScript :=
			classifyViteBuildScript(
				strings.TrimSpace(manifest.Scripts["build"]),
			)
		if !supportedBuildScript {
			return deploymentBuildPlan{}, fmt.Errorf(
				"persisted Vite strategy requires a supported build script",
			)
		}

		if record.PackageInstallMode == packageInstallModeCI {
			if _, err := validatedPackageLock(repositoryPath); err != nil {
				return deploymentBuildPlan{}, fmt.Errorf(
					"persisted npm ci strategy: %w",
					err,
				)
			}
		}

		if record.PackageInstallMode != packageInstallModeCI &&
			record.PackageInstallMode != packageInstallModeSetup {
			return deploymentBuildPlan{}, fmt.Errorf(
				"unsupported persisted npm install mode %q",
				record.PackageInstallMode,
			)
		}

		return viteStaticBuildPlan(
			record.PackageInstallMode,
			appendBuildSubcommand,
		), nil

	case deploymentStrategyNodeExpress:
		if record.PackageManager != packageManagerNPM {
			return deploymentBuildPlan{}, fmt.Errorf(
				"unsupported persisted package manager %q",
				record.PackageManager,
			)
		}

		manifest, found, err := readNodePackageManifest(
			repositoryPath,
		)
		if err != nil {
			return deploymentBuildPlan{}, err
		} else if !found {
			return deploymentBuildPlan{}, fmt.Errorf(
				"persisted Node/Express strategy requires package.json",
			)
		}

		conventionalJavaScript, err :=
			repositoryIsConventionalJavaScriptExpress(
				repositoryPath,
				manifest,
			)
		if err != nil {
			return deploymentBuildPlan{}, err
		}

		if !conventionalJavaScript {
			return deploymentBuildPlan{}, fmt.Errorf(
				"persisted Node/Express strategy requires a conventional JavaScript Express repository",
			)
		}

		expressVersion, hasExpress := manifest.Dependencies["express"]
		if !hasExpress || strings.TrimSpace(expressVersion) == "" {
			return deploymentBuildPlan{}, fmt.Errorf(
				"persisted Node/Express strategy requires express as a runtime dependency",
			)
		}

		supportedEntrypoint, err := repositoryHasSupportedNodeEntrypoint(
			repositoryPath,
			manifest.Scripts["start"],
		)
		if err != nil {
			return deploymentBuildPlan{}, err
		}

		if !supportedEntrypoint {
			return deploymentBuildPlan{}, fmt.Errorf(
				"persisted Node/Express strategy requires a supported direct Node entrypoint",
			)
		}

		usesNPM, err := repositoryUsesNPM(
			repositoryPath,
			manifest,
		)
		if err != nil {
			return deploymentBuildPlan{}, err
		}

		if !usesNPM {
			return deploymentBuildPlan{}, fmt.Errorf(
				"persisted Node/Express strategy requires npm",
			)
		}

		if record.PackageInstallMode == packageInstallModeCI {
			if _, err := validatedPackageLock(repositoryPath); err != nil {
				return deploymentBuildPlan{}, fmt.Errorf(
					"persisted npm ci strategy: %w",
					err,
				)
			}
		}

		if record.PackageInstallMode != packageInstallModeCI &&
			record.PackageInstallMode != packageInstallModeSetup {

			return deploymentBuildPlan{}, fmt.Errorf(
				"unsupported persisted npm install mode %q",
				record.PackageInstallMode,
			)
		}

		plan := nodeExpressBuildPlan(
			record.PackageInstallMode,
			record.ContainerPort,
			record.HealthPath,
		)
		plan.ReactorLabMigration = strings.TrimSpace(manifest.Scripts["reactorlab:migrate"]) != ""
		return plan, nil

	default:
		return deploymentBuildPlan{}, fmt.Errorf(
			"unsupported persisted deployment strategy %q",
			record.Strategy,
		)
	}
}

func readNodePackageManifest(
	repositoryPath string,
) (nodePackageManifest, bool, error) {
	manifestPath, found, err := repositoryRegularFile(
		repositoryPath,
		"package.json",
	)
	if err != nil {
		return nodePackageManifest{}, false, fmt.Errorf(
			"inspect package.json: %w",
			err,
		)
	}
	if !found {
		return nodePackageManifest{}, false, nil
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nodePackageManifest{}, false, fmt.Errorf(
			"read package.json: %w",
			err,
		)
	}

	var manifest nodePackageManifest

	if err := json.Unmarshal(data, &manifest); err != nil {
		return nodePackageManifest{}, false, fmt.Errorf(
			"parse package.json: %w",
			err,
		)
	}

	return manifest, true, nil
}

func hasNodeDependency(
	manifest nodePackageManifest,
	name string,
) bool {
	if _, ok := manifest.Dependencies[name]; ok {
		return true
	}

	_, ok := manifest.DevDependencies[name]
	return ok
}

func repositoryHasViteConfig(
	repositoryPath string,
) (bool, error) {
	for _, name := range []string{
		"vite.config.js",
		"vite.config.mjs",
		"vite.config.cjs",
		"vite.config.ts",
		"vite.config.mts",
		"vite.config.cts",
	} {
		_, found, err := repositoryRegularFile(repositoryPath, name)
		if err != nil {
			return false, fmt.Errorf(
				"inspect %s: %w",
				name,
				err,
			)
		}
		if found {
			return true, nil
		}
	}

	return false, nil
}

func npmInstallMode(
	repositoryPath string,
) (string, error) {
	_, found, err := repositoryRegularFile(
		repositoryPath,
		"package-lock.json",
	)
	if err != nil {
		return "", fmt.Errorf(
			"inspect package-lock.json: %w",
			err,
		)
	}
	if !found {
		return packageInstallModeSetup, nil
	}

	if _, err := validatedPackageLock(repositoryPath); err != nil {
		return "", err
	}

	return packageInstallModeCI, nil
}

func validatedPackageLock(
	repositoryPath string,
) (npmPackageLock, error) {
	lockPath, found, err := repositoryRegularFile(
		repositoryPath,
		"package-lock.json",
	)
	if err != nil {
		return npmPackageLock{}, fmt.Errorf(
			"inspect package-lock.json: %w",
			err,
		)
	}
	if !found {
		return npmPackageLock{}, fmt.Errorf("package-lock.json is missing")
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return npmPackageLock{}, fmt.Errorf(
			"read package-lock.json: %w",
			err,
		)
	}

	var lock npmPackageLock

	if err := json.Unmarshal(data, &lock); err != nil {
		return npmPackageLock{}, fmt.Errorf(
			"parse package-lock.json: %w",
			err,
		)
	}

	if lock.LockfileVersion < 1 {
		return npmPackageLock{}, fmt.Errorf(
			"package-lock.json has no supported lockfileVersion",
		)
	}

	return lock, nil
}

func viteStaticBuildPlan(
	installMode string,
	appendBuildSubcommand bool,
) deploymentBuildPlan {
	return deploymentBuildPlan{
		Strategy:           deploymentStrategyViteStatic,
		PackageManager:     packageManagerNPM,
		PackageInstallMode: installMode,
		ContainerPort:      80,
		HealthPath:         "/",
		GeneratedDockerfile: generatedViteDockerfile(
			installMode,
			appendBuildSubcommand,
		),
	}
}

func generatedViteDockerfile(
	installMode string,
	appendBuildSubcommand bool,
) string {
	installCommand := "npm install --no-audit --no-fund"
	if installMode == packageInstallModeCI {
		installCommand = "npm ci --no-audit --no-fund"
	}

	buildCommand := "npm run build -- --base=/ --outDir=dist"
	if appendBuildSubcommand {
		buildCommand = "npm run build -- build --base=/ --outDir=dist"
	}

	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM node:24-alpine AS build

WORKDIR /app
COPY package*.json ./
RUN %s
COPY . .
RUN %s

FROM nginx:stable-alpine AS runtime
COPY --from=build /app/dist/ /usr/share/nginx/html/
RUN printf '%%s\n' \
    'server {' \
    '    listen 80;' \
    '    server_name _;' \
    '    root /usr/share/nginx/html;' \
    '    index index.html;' \
    '    location / {' \
    '        try_files $uri $uri/ /index.html;' \
    '    }' \
    '}' > /etc/nginx/conf.d/default.conf

EXPOSE 80
`, installCommand, buildCommand)
}

func nodeExpressBuildPlan(
	installMode string,
	containerPort int,
	healthPath string,
) deploymentBuildPlan {
	return deploymentBuildPlan{
		Strategy:           deploymentStrategyNodeExpress,
		PackageManager:     packageManagerNPM,
		PackageInstallMode: installMode,
		ContainerPort:      containerPort,
		HealthPath:         healthPath,
		GeneratedDockerfile: generatedNodeExpressDockerfile(
			installMode,
			containerPort,
		),
	}
}

func generatedNodeExpressDockerfile(
	installMode string,
	containerPort int,
) string {
	installCommand := "npm install --no-audit --no-fund"
	if installMode == packageInstallModeCI {
		installCommand = "npm ci --no-audit --no-fund"
	}

	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM node:24-alpine

WORKDIR /app
COPY package*.json ./
COPY . .
RUN %s

EXPOSE %d
CMD ["npm", "start"]
`, installCommand, containerPort)
}

func buildDeploymentImage(
	repositoryPath string,
	imageName string,
	plan deploymentBuildPlan,
) (string, error) {
	if plan.GeneratedDockerfile == "" {
		return runCommand(
			repositoryPath,
			"docker",
			"build",
			"-t",
			imageName,
			".",
		)
	}

	buildRoot := os.TempDir()
	buildDirectory, err := os.MkdirTemp(
		buildRoot,
		"minideploy-build-",
	)
	if err != nil {
		return "", fmt.Errorf(
			"create generated build context: %w",
			err,
		)
	}
	defer func() {
		_ = removeStrictChildPath(
			buildRoot,
			buildDirectory,
		)
	}()

	dockerfilePath := filepath.Join(
		buildDirectory,
		"Dockerfile",
	)

	if err := os.WriteFile(
		dockerfilePath,
		[]byte(plan.GeneratedDockerfile),
		0600,
	); err != nil {
		return "", fmt.Errorf(
			"write generated Dockerfile: %w",
			err,
		)
	}

	return runCommand(
		"",
		"docker",
		"build",
		"-t",
		imageName,
		"-f",
		dockerfilePath,
		repositoryPath,
	)
}

func describeDeploymentPlan(
	app string,
	plan deploymentBuildPlan,
) {
	switch plan.Strategy {
	case deploymentStrategyDockerfile:
		deploymentEvent(
			app,
			"Detected Dockerfile deployment strategy.",
		)

	case deploymentStrategyFullstackViteNode:
		deploymentEvent(
			app,
			"Detected full-stack Vite + Node/Express project.",
		)
		for _, service := range plan.Services {
			deploymentEvent(
				app,
				"%s: detected %s in %s/.",
				fullstackServiceDisplayName(service.Name),
				service.Build.Strategy,
				service.Path,
			)
		}

	case deploymentStrategyViteStatic:
		deploymentEvent(
			app,
			"Detected React/Vite project.",
		)

		if plan.PackageInstallMode == packageInstallModeCI {
			deploymentEvent(
				app,
				"Installing npm dependencies with npm ci during image build...",
			)
		} else {
			deploymentEvent(
				app,
				"Installing npm dependencies with npm install during image build...",
			)
		}

		deploymentEvent(
			app,
			"Building production bundle with npm run build...",
		)
		deploymentEvent(
			app,
			"Packaging static runtime with SPA fallback...",
		)

	case deploymentStrategyNodeExpress:
		deploymentEvent(
			app,
			"Detected conventional JavaScript Node/Express service.",
		)

		if plan.PackageInstallMode == packageInstallModeCI {
			deploymentEvent(
				app,
				"Installing npm dependencies with npm ci during image build...",
			)
		} else {
			deploymentEvent(
				app,
				"Installing npm dependencies with npm install during image build...",
			)
		}

		deploymentEvent(
			app,
			"Runtime will start with npm start and MiniDeploy-managed PORT.",
		)
	}
}
