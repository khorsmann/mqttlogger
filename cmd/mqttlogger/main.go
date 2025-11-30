package main

import (
	"fmt"
	"log"
	"os"

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

	// -------------------------------
	// CLI: backup / restore
	// -------------------------------
	if len(os.Args) > 2 {
		cmd := os.Args[1]
		argPath := os.Args[2]

		switch cmd {

		case "backup":
			source := cfg.Database.Path
			target := argPath

			database, err := db.Open(source)
			if err != nil {
				log.Fatal(err)
			}
			defer database.Close()

			if err := db.CreateBackup(database, source, target); err != nil {
				log.Fatalf("Backup fehlgeschlagen: %v", err)
			}

			fmt.Println("✔ Backup erfolgreich erstellt:", target)
			os.Exit(0)

		case "restore":
			source := argPath
			target := cfg.Database.Path

			if err := db.RestoreBackup(target, source); err != nil {
				log.Fatalf("Restore fehlgeschlagen: %v", err)
			}

			fmt.Println("✔ Restore erfolgreich. Bitte Dienst neu starten.")
			os.Exit(0)
		}
	}

	// -------------------------------
	// Normale Ausführung
	// -------------------------------

	// DB öffnen
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Fehler beim Öffnen der Datenbank: %v", err)
	}
	defer database.Close()

	// Tabellen & Views & Aggregationen initialisieren
	if err := db.InitDB(database, cfg); err != nil {
		log.Fatalf("Fehler beim Initialisieren der DB: %v", err)
	}

	// MQTT starten
	mqtt.StartClient(cfg, database)

	// Endlosschleife
	select {}
}
