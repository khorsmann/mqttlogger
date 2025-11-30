package main

import (
	"fmt"
	"log"
	"os"

	"github.com/khorsmann/mqttlogger/internal/cli"
	"github.com/khorsmann/mqttlogger/internal/config"
	"github.com/khorsmann/mqttlogger/internal/db"
	"github.com/khorsmann/mqttlogger/internal/mqtt"
)

func printHelp() {
	cli.Bold("MQTTLOGGER – Befehle:")
	fmt.Print(`
  mqttlogger backup <file>    - erstellt ein Backup
  mqttlogger restore <file>   - stellt eine DB wieder her
  --verbose                   - zeigt Details während der Ausführung
  --debug                     - SQL-Kommandos anzeigen
  --help                      - diese Hilfe
`)
}

func main() {

	if len(os.Args) > 1 && os.Args[1] == "--help" {
		printHelp()
		return
	}

	// Flags
	verbose := contains(os.Args, "--verbose")
	debug := contains(os.Args, "--debug")

	cfg, err := config.Load("config.toml")
	if err != nil {
		log.Fatalf("Fehler beim Laden der Config: %v", err)
	}

	// CLI-Befehle
	if len(os.Args) > 2 {
		command := os.Args[1]
		path := os.Args[2]

		switch command {

		case "backup":
			dbh, err := db.Open(cfg.Database.Path)
			if err != nil {
				cli.Error("Konnte DB nicht öffnen.")
				os.Exit(1)
			}
			defer dbh.Close()

			if err := db.CreateBackup(dbh, cfg.Database.Path, path, verbose, debug); err != nil {
				cli.Error("Backup fehlgeschlagen: " + err.Error())
				os.Exit(1)
			}

			cli.Success("Backup erstellt: " + path)
			os.Exit(0)

		case "restore":
			if err := db.RestoreBackup(cfg.Database.Path, path, verbose, debug); err != nil {
				cli.Error("Restore fehlgeschlagen: " + err.Error())
				os.Exit(1)
			}

			cli.Success("Restore erfolgreich. Bitte Dienst neu starten!")
			os.Exit(0)
		}
	}

	// Normaler Betrieb
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Fehler beim Öffnen der DB: %v", err)
	}
	defer database.Close()

	if err := db.InitDB(database, cfg); err != nil {
		log.Fatalf("Fehler beim Initialisieren der DB: %v", err)
	}

	mqtt.StartClient(cfg, database)
	select {}
}

// Helper
func contains(list []string, val string) bool {
	for _, v := range list {
		if v == val {
			return true
		}
	}
	return false
}
