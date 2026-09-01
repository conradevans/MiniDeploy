package main

type HealthResponse struct {
	Status string `json:"status"`
}

type GuestDeploymentResponse struct {
	App    string `json:"app"`
	URL    string `json:"url"`
	Status string `json:"status"`
}

type AdminSessionResponse struct {
	Role  string `json:"role"`
	Email string `json:"email"`
}

type DeployRequest struct {
	RepoURL       string            `json:"repoUrl"`
	ContainerPort int               `json:"containerPort"`
	HealthPath    string            `json:"healthPath"`
	Environment   map[string]string `json:"environment"`
}

type RedeployRequest struct {
	Environment map[string]string `json:"environment"`
}

type DeploymentResponse struct {
	App                  string                      `json:"app"`
	RepoURL              string                      `json:"repoUrl"`
	Container            string                      `json:"container"`
	Image                string                      `json:"image"`
	Port                 int                         `json:"port"`
	ContainerPort        int                         `json:"containerPort"`
	HealthPath           string                      `json:"healthPath"`
	Strategy             string                      `json:"strategy"`
	PackageManager       string                      `json:"packageManager,omitempty"`
	EnvironmentVariables []string                    `json:"environmentVariables,omitempty"`
	Services             []DeploymentServiceResponse `json:"services,omitempty"`

	Status string `json:"status"`
}

type DeploymentServiceResponse struct {
	Name               string `json:"name"`
	Path               string `json:"path"`
	Strategy           string `json:"strategy"`
	Container          string `json:"container"`
	Image              string `json:"image"`
	Port               int    `json:"port"`
	ContainerPort      int    `json:"containerPort"`
	HealthPath         string `json:"healthPath"`
	PackageManager     string `json:"packageManager,omitempty"`
	PackageInstallMode string `json:"packageInstallMode,omitempty"`
	Status             string `json:"status"`
}

type LogsResponse struct {
	App       string `json:"app"`
	Container string `json:"container"`
	Logs      string `json:"logs"`
}

type DeploymentLogsResponse struct {
	App  string `json:"app"`
	Logs string `json:"logs"`
}

type ActionResponse struct {
	Status    string `json:"status"`
	App       string `json:"app"`
	Container string `json:"container"`
}

type HistoryResponse struct {
	App      string              `json:"app"`
	Versions []DeploymentVersion `json:"versions"`
}
