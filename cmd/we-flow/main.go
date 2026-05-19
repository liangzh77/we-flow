package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"we-flow/internal/server"
	"we-flow/internal/store"
	"we-flow/internal/wx"
)

func main() {
	addr := env("WE_FLOW_ADDR", "127.0.0.1:8787")
	dbPath := env("WE_FLOW_DB", "data/we-flow.db")

	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer db.Close()

	wxClient := wx.New("wx", 45*time.Second)
	app := server.New(db, wxClient, "web")

	log.Printf("we-flow listening on http://%s", addr)
	if err := http.ListenAndServe(addr, app.Routes()); err != nil {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
