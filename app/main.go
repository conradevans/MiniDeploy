package main

import (
	"log"
	"net/http"
)

func main() {
	if err := syncProxyRoutes(); err != nil {
		log.Printf(
			"warning: initial proxy sync failed: %v",
			err,
		)
	}

	address := "127.0.0.1:9000"

	log.Printf(
		"MiniDeploy API listening on http://%s",
		address,
	)

	if err := http.ListenAndServe(
		address,
		routes(),
	); err != nil {
		log.Fatal(err)
	}
}
