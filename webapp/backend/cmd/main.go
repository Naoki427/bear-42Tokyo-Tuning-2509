package main

import (
	"backend/internal/server"
	"log"
	"net/http"
	_ "net/http/pprof"
	"backend/internal/telemetry"
	"context"
)

func main() {
	srv, dbConn, err := server.NewServer()
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}
	shutdown, err := telemetry.Init(context.Background())
	if err != nil {
		log.Printf("telemetry init failed: %v, continuing without telemetry", err)
	} else {
		defer func() { _ = shutdown(context.Background()) }()
	}
	if dbConn != nil {
		defer dbConn.Close()
	}

	go func() {
		log.Println("pprof server starting on :6060")
		log.Println(http.ListenAndServe(":6060", nil))
	}()

	srv.Run()
}
