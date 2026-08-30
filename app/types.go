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
	RepoURL       string `json:"repoUrl"`
	ContainerPort int    `json:"containerPort"`
	HealthPath    string `json:"healthPath"`
}

type DeploymentResponse struct {
	App           string `json:"app"`
	RepoURL       string `json:"repoUrl"`
	Container     string `json:"container"`
	Image         string `json:"image"`
	Port          int    `json:"port"`
	ContainerPort int    `json:"containerPort"`
	HealthPath    string `json:"healthPath"`

	Status string `json:"status"`
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
