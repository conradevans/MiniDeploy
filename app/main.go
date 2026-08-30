package main

import (
	"errors"
	"log"
	"net/http"
	"time"
)

func main() {
	if err := syncProxyRoutes(); err != nil {
		log.Printf(
			"warning: initial proxy sync failed: %v",
			err,
		)
	}

	address := "127.0.0.1:9000"

	server := &http.Server{
		Addr:              address,
		Handler:           securityMiddleware(routes()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}

	log.Printf(
		"MiniDeploy API listening on http://%s",
		address,
	)

	if err := server.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {

		log.Fatal(err)
	}
}
