package main

import (
	"log"

	"github.com/khorsmann/mqttlogger/internal/config"
	"github.com/khorsmann/mqttlogger/internal/db"
	"github.com/khorsmann/mqttlogger/internal/mqtt"
)

func main() {
	// Config laden
	cfg, err := config.Load("config.toml")
	if err != nil {
		log.Fatalf("Fehler beim Laden der Config: %v", err)
	}

	// DB öffnen
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Fehler beim Öffnen der Datenbank: %v", err)
	}
	defer database.Close()

	// Tabellen & Views erstellen
	if err := db.InitDB(database, cfg); err != nil {
		log.Fatalf("Fehler beim Initialisieren der DB: %v", err)
	}

	// MQTT starten
	_ = mqtt.StartClient(cfg, database)

	// Endlosschleife (MQTT + Aggregation laufen in Hintergrund-Goroutines)
	select {}
}
