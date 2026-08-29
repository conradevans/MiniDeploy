package main

type HealthResponse struct {
	Status string `json:"status"`
}

type DeployRequest struct {
	RepoURL string `json:"repoUrl"`
}

type DeploymentResponse struct {
	App       string `json:"app"`
	RepoURL   string `json:"repoUrl"`
	Container string `json:"container"`
	Image     string `json:"image"`
	Port      int    `json:"port"`
	Status    string `json:"status"`
}

type LogsResponse struct {
	App       string `json:"app"`
	Container string `json:"container"`
	Logs      string `json:"logs"`
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
