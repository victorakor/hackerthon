package main

import (
	app "hackathon/go"
	"log"
)

func main() {
	if err := app.InitDB(); err != nil {
		log.Fatalf("DB init failed: %v", err)
	}
	defer app.DB.Close()
	app.RegisterRoutes()
	app.StartServer()
}
