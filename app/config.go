package main

import (
	"sync"
)

const (
	metadataPath  = "/srv/minideploy/data/deployments.json"
	minDeployPort = 8081
	maxDeployPort = 8999
)

var (
	deploymentsDir = "/srv/minideploy/managed-deployments"
	deployMu       sync.Mutex

	store DeploymentStore = NewJSONStore(
		metadataPath,
	)

	historyStore = NewJSONHistoryStore(
		"/srv/minideploy/data/deployment-history.json",
		3,
	)
)
