package main

import (
	"log"
	"net/http"
)

func main() {
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
