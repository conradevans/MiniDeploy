package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	managementAddress = "127.0.0.1:9000"
	publicAddress     = "127.0.0.1:9003"
)

func main() {
	if err := syncProxyRoutes(); err != nil {
		log.Printf(
			"warning: initial proxy sync failed: %v",
			err,
		)
	}

	accessValidator, err := newCloudflareAccessValidator(
		accessConfigFromEnvironment(),
	)
	if err != nil {
		log.Printf(
			"warning: public admin routes disabled: %v",
			err,
		)
		accessValidator = nil
	}

	managementServer := &http.Server{
		Addr:              managementAddress,
		Handler:           securityMiddleware(routes()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}

	publicServer := &http.Server{
		Addr: publicAddress,
		Handler: publicSecurityMiddleware(
			publicRoutes(accessValidator),
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}

	log.Printf(
		"MiniDeploy API listening on http://%s",
		managementAddress,
	)

	log.Printf(
		"MiniDeploy public origin listening on http://%s",
		publicAddress,
	)

	serverErrors := make(chan error, 2)

	go serveHTTP(managementServer, serverErrors)
	go serveHTTP(publicServer, serverErrors)

	log.Fatal(<-serverErrors)
}

func serveHTTP(
	server *http.Server,
	errorsChannel chan<- error,
) {
	if err := server.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {

		errorsChannel <- fmt.Errorf(
			"listen on %s: %w",
			server.Addr,
			err,
		)
	}
}
