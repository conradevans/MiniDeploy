package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const reactorLabHealthTimeout = 750 * time.Millisecond

type ReactorLabObservabilityResponse struct {
	Deployments []ReactorLabDeploymentMetrics `json:"deployments"`
	CollectedAt time.Time                     `json:"collectedAt"`
}

type ReactorLabDeploymentMetrics struct {
	App        string                       `json:"app"`
	Strategy   string                       `json:"strategy"`
	Status     string                       `json:"status"`
	Containers []ReactorLabContainerMetrics `json:"containers"`
}

type ReactorLabContainerMetrics struct {
	Service          string  `json:"service"`
	Strategy         string  `json:"strategy"`
	Container        string  `json:"container"`
	State            string  `json:"state"`
	Health           string  `json:"health"`
	CPUPercent       float64 `json:"cpuPercent"`
	MemoryUsedBytes  uint64  `json:"memoryUsedBytes"`
	MemoryLimitBytes uint64  `json:"memoryLimitBytes"`
	MemoryPercent    float64 `json:"memoryPercent"`
	NetworkRXBytes   uint64  `json:"networkRxBytes"`
	NetworkTXBytes   uint64  `json:"networkTxBytes"`
	BlockReadBytes   uint64  `json:"blockReadBytes"`
	BlockWriteBytes  uint64  `json:"blockWriteBytes"`
	PIDs             int     `json:"pids"`
	WritableBytes    int64   `json:"writableBytes"`
	UptimeSeconds    float64 `json:"uptimeSeconds"`
	RestartCount     int     `json:"restartCount"`
}

type reactorLabTarget struct {
	App        string
	Service    string
	Strategy   string
	Container  string
	Port       int
	HealthPath string
}

type dockerInspectState struct {
	Status    string `json:"Status"`
	StartedAt string `json:"StartedAt"`
}

type dockerInspectMetrics struct {
	State         string
	StartedAt     time.Time
	RestartCount  int
	WritableBytes int64
}

type dockerStatsRow struct {
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
	MemPerc  string `json:"MemPerc"`
	NetIO    string `json:"NetIO"`
	BlockIO  string `json:"BlockIO"`
	PIDs     string `json:"PIDs"`
}

type dockerRuntimeStats struct {
	CPUPercent       float64
	MemoryUsedBytes  uint64
	MemoryLimitBytes uint64
	MemoryPercent    float64
	NetworkRXBytes   uint64
	NetworkTXBytes   uint64
	BlockReadBytes   uint64
	BlockWriteBytes  uint64
	PIDs             int
}

func reactorLabDeploymentsHandler(w http.ResponseWriter, _ *http.Request) {
	response, err := collectReactorLabObservability()
	if err != nil {
		http.Error(
			w,
			"deployment observability unavailable",
			http.StatusServiceUnavailable,
		)
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func collectReactorLabObservability() (ReactorLabObservabilityResponse, error) {
	records, err := store.List()
	if err != nil {
		return ReactorLabObservabilityResponse{}, fmt.Errorf(
			"list deployments: %w",
			err,
		)
	}

	targetsByApp := make(map[string][]reactorLabTarget, len(records))
	targetsByContainer := make(map[string]reactorLabTarget)

	for _, record := range records {
		targets := reactorLabTargets(record)
		targetsByApp[record.App] = targets
		for _, target := range targets {
			if target.Container != "" {
				targetsByContainer[target.Container] = target
			}
		}
	}

	existing, err := dockerContainerNames()
	if err != nil {
		return ReactorLabObservabilityResponse{}, err
	}

	existingTargets := make([]string, 0, len(targetsByContainer))
	for name := range targetsByContainer {
		if existing[name] {
			existingTargets = append(existingTargets, name)
		}
	}

	inspect, err := dockerInspectObservability(existingTargets)
	if err != nil {
		return ReactorLabObservabilityResponse{}, err
	}

	running := make([]string, 0, len(inspect))
	for name, metrics := range inspect {
		if metrics.State == "running" {
			running = append(running, name)
		}
	}

	stats, err := dockerStatsObservability(running)
	if err != nil {
		return ReactorLabObservabilityResponse{}, err
	}

	collectedAt := time.Now().UTC()
	deployments := make(
		[]ReactorLabDeploymentMetrics,
		0,
		len(records),
	)

	for _, record := range records {
		targets := targetsByApp[record.App]
		containers := make(
			[]ReactorLabContainerMetrics,
			0,
			len(targets),
		)

		for _, target := range targets {
			container := ReactorLabContainerMetrics{
				Service:   target.Service,
				Strategy:  target.Strategy,
				Container: target.Container,
				State:     "missing",
				Health:    "unavailable",
			}

			if inspected, ok := inspect[target.Container]; ok {
				container.State = inspected.State
				container.RestartCount = inspected.RestartCount
				container.WritableBytes = inspected.WritableBytes

				if inspected.State == "running" &&
					!inspected.StartedAt.IsZero() {

					uptime := collectedAt.Sub(inspected.StartedAt).Seconds()
					if uptime > 0 {
						container.UptimeSeconds = uptime
					}
				}

				if runtimeStats, ok := stats[target.Container]; ok {
					container.CPUPercent = runtimeStats.CPUPercent
					container.MemoryUsedBytes = runtimeStats.MemoryUsedBytes
					container.MemoryLimitBytes = runtimeStats.MemoryLimitBytes
					container.MemoryPercent = runtimeStats.MemoryPercent
					container.NetworkRXBytes = runtimeStats.NetworkRXBytes
					container.NetworkTXBytes = runtimeStats.NetworkTXBytes
					container.BlockReadBytes = runtimeStats.BlockReadBytes
					container.BlockWriteBytes = runtimeStats.BlockWriteBytes
					container.PIDs = runtimeStats.PIDs
				}

				if inspected.State == "running" {
					container.Health = reactorLabHTTPHealth(target)
				}
			}

			containers = append(containers, container)
		}

		deployments = append(
			deployments,
			ReactorLabDeploymentMetrics{
				App:        record.App,
				Strategy:   reactorLabStrategy(record.Strategy),
				Status:     reactorLabDeploymentStatus(record, containers),
				Containers: containers,
			},
		)
	}

	return ReactorLabObservabilityResponse{
		Deployments: deployments,
		CollectedAt: collectedAt,
	}, nil
}

func reactorLabStrategy(strategy string) string {
	if strategy == "" {
		return deploymentStrategyDockerfile
	}
	return strategy
}

func reactorLabTargets(record DeploymentRecord) []reactorLabTarget {
	if len(record.Services) == 0 {
		return []reactorLabTarget{{
			App:        record.App,
			Service:    "app",
			Strategy:   reactorLabStrategy(record.Strategy),
			Container:  record.Container,
			Port:       record.Port,
			HealthPath: record.HealthPath,
		}}
	}

	targets := make([]reactorLabTarget, 0, len(record.Services))
	for _, service := range record.Services {
		targets = append(targets, reactorLabTarget{
			App:        record.App,
			Service:    service.Name,
			Strategy:   service.Strategy,
			Container:  service.Container,
			Port:       service.Port,
			HealthPath: service.HealthPath,
		})
	}
	return targets
}

func reactorLabDeploymentStatus(
	record DeploymentRecord,
	containers []ReactorLabContainerMetrics,
) string {
	if record.DatabaseDetached {
		return "database-detached"
	}

	if len(containers) == 0 {
		return "unavailable"
	}

	allHealthy := true
	anyRunning := false

	for _, container := range containers {
		if container.State == "running" {
			anyRunning = true
		}
		if container.State != "running" || container.Health != "healthy" {
			allHealthy = false
		}
	}

	if allHealthy {
		return "healthy"
	}
	if anyRunning {
		return "degraded"
	}
	return "unavailable"
}

func dockerContainerNames() (map[string]bool, error) {
	output, err := runCommand(
		"",
		"docker",
		"ps",
		"-a",
		"--format",
		"{{.Names}}",
	)
	if err != nil {
		return nil, fmt.Errorf("list Docker containers: %w", err)
	}

	names := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names[name] = true
		}
	}
	return names, nil
}

func dockerInspectObservability(
	names []string,
) (map[string]dockerInspectMetrics, error) {
	result := map[string]dockerInspectMetrics{}
	if len(names) == 0 {
		return result, nil
	}

	format := "{{json .State}}\t{{.Name}}\t{{.RestartCount}}\t{{.SizeRw}}"
	args := []string{"inspect", "--size", "--format", format}
	args = append(args, names...)

	output, err := runCommand("", "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("inspect Docker containers: %w", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("invalid Docker inspect observability row")
		}

		var state dockerInspectState
		if err := json.Unmarshal([]byte(parts[0]), &state); err != nil {
			return nil, fmt.Errorf("decode Docker state: %w", err)
		}

		restartCount, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			return nil, fmt.Errorf("parse Docker restart count: %w", err)
		}

		var writableBytes int64
		sizeText := strings.TrimSpace(parts[3])
		if sizeText != "" && sizeText != "<nil>" {
			writableBytes, err = strconv.ParseInt(sizeText, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse Docker writable size: %w", err)
			}
		}

		var startedAt time.Time
		if state.StartedAt != "" {
			startedAt, _ = time.Parse(time.RFC3339Nano, state.StartedAt)
		}

		name := strings.TrimPrefix(strings.TrimSpace(parts[1]), "/")
		result[name] = dockerInspectMetrics{
			State:         state.Status,
			StartedAt:     startedAt,
			RestartCount:  restartCount,
			WritableBytes: writableBytes,
		}
	}

	return result, nil
}

func dockerStatsObservability(
	names []string,
) (map[string]dockerRuntimeStats, error) {
	result := map[string]dockerRuntimeStats{}
	if len(names) == 0 {
		return result, nil
	}

	args := []string{"stats", "--no-stream", "--format", "{{json .}}"}
	args = append(args, names...)

	output, err := runCommand("", "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("collect Docker stats: %w", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var row dockerStatsRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("decode Docker stats: %w", err)
		}

		cpuPercent, err := parseDockerPercent(row.CPUPerc)
		if err != nil {
			return nil, err
		}
		memoryPercent, err := parseDockerPercent(row.MemPerc)
		if err != nil {
			return nil, err
		}
		memoryUsed, memoryLimit, err := parseDockerBytePair(row.MemUsage)
		if err != nil {
			return nil, err
		}
		networkRX, networkTX, err := parseDockerBytePair(row.NetIO)
		if err != nil {
			return nil, err
		}
		blockRead, blockWrite, err := parseDockerBytePair(row.BlockIO)
		if err != nil {
			return nil, err
		}
		pids, err := strconv.Atoi(strings.TrimSpace(row.PIDs))
		if err != nil {
			return nil, fmt.Errorf("parse Docker PIDs: %w", err)
		}

		result[row.Name] = dockerRuntimeStats{
			CPUPercent:       cpuPercent,
			MemoryUsedBytes:  memoryUsed,
			MemoryLimitBytes: memoryLimit,
			MemoryPercent:    memoryPercent,
			NetworkRXBytes:   networkRX,
			NetworkTXBytes:   networkTX,
			BlockReadBytes:   blockRead,
			BlockWriteBytes:  blockWrite,
			PIDs:             pids,
		}
	}

	return result, nil
}

func parseDockerPercent(value string) (float64, error) {
	value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse Docker percent %q: %w", value, err)
	}
	return parsed, nil
}

func parseDockerBytePair(value string) (uint64, uint64, error) {
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid Docker byte pair %q", value)
	}

	left, err := parseDockerBytes(parts[0])
	if err != nil {
		return 0, 0, err
	}
	right, err := parseDockerBytes(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return left, right, nil
}

func parseDockerBytes(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	i := 0
	digits := 0
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		i++
		digits++
	}
	if i < len(value) && value[i] == '.' {
		i++
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return 0, fmt.Errorf("invalid Docker byte value %q", value)
	}
	if i < len(value) && (value[i] == 'e' || value[i] == 'E') {
		i++
		if i < len(value) && (value[i] == '+' || value[i] == '-') {
			i++
		}
		exponentStart := i
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
		}
		if i == exponentStart {
			return 0, fmt.Errorf("invalid Docker byte exponent %q", value)
		}
	}

	number, err := strconv.ParseFloat(value[:i], 64)
	if err != nil {
		return 0, fmt.Errorf("parse Docker byte number %q: %w", value, err)
	}

	unit := strings.TrimSpace(value[i:])
	multipliers := map[string]float64{
		"B":   1,
		"kB":  1e3,
		"MB":  1e6,
		"GB":  1e9,
		"TB":  1e12,
		"KiB": 1 << 10,
		"MiB": 1 << 20,
		"GiB": 1 << 30,
		"TiB": 1 << 40,
	}

	multiplier, ok := multipliers[unit]
	if !ok {
		return 0, fmt.Errorf("unsupported Docker byte unit %q", unit)
	}

	return uint64(number * multiplier), nil
}

func reactorLabHTTPHealth(target reactorLabTarget) string {
	if target.Port <= 0 {
		return "unavailable"
	}

	client := &http.Client{
		Timeout: reactorLabHealthTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	url := fmt.Sprintf(
		"http://127.0.0.1:%d%s",
		target.Port,
		normalizedHealthPath(target.HealthPath),
	)

	response, err := client.Get(url)
	if err != nil {
		return "unhealthy"
	}
	response.Body.Close()

	if response.StatusCode >= 200 && response.StatusCode < 400 {
		return "healthy"
	}
	return "unhealthy"
}
