package db

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/khorsmann/mqttlogger/internal/config"

	_ "github.com/mattn/go-sqlite3"
)

// Open öffnet die SQLite-Datenbank und setzt journal_mode auf WAL
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	return db, nil
}

// InitDB erstellt Tabellen, Views und startet Aggregation
func InitDB(db *sql.DB, cfg config.Config) error {
	if err := createTables(db); err != nil {
		return err
	}
	if err := createViews(db, cfg.Cost.PerKWh); err != nil {
		return err
	}
	startAggregation(db, cfg)
	return nil
}

// ---------------- Tabellen & Views ----------------

func createTables(db *sql.DB) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS energy_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			e_in REAL,
			e_out REAL,
			power INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS tasmota_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			power INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS solar_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			device_id TEXT,
			channel INTEGER,
			metric TEXT,
			value REAL
		);`,
		`CREATE TABLE IF NOT EXISTS solar_meta (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT,
			channel INTEGER,
			key TEXT,
			value TEXT,
			UNIQUE(device_id, channel, key)
		);`,
	}

	for _, t := range tables {
		if _, err := db.Exec(t); err != nil {
			return fmt.Errorf("Fehler beim Erstellen der Tabelle: %w", err)
		}
	}
	return nil
}

func createViews(db *sql.DB, costPerKWh float64) error {
	views := []string{
		fmt.Sprintf(`CREATE VIEW IF NOT EXISTS energy_cost AS
			SELECT timestamp_unix, timestamp_rfc3339, e_in, e_out,
			(e_in - e_out) * %.4f AS cost
			FROM energy_data;`, costPerKWh),
	}

	for _, v := range views {
		if _, err := db.Exec(v); err != nil {
			return fmt.Errorf("Fehler beim Erstellen der View: %w", err)
		}
	}
	return nil
}

// ---------------- Aggregation ----------------

func startAggregation(db *sql.DB, cfg config.Config) {
	log.Println("Aggregation gestartet (hier können deine Stunden-/Tages-/Monats-Aggregationen eingebaut werden)")
	// Beispiel: In Zukunft könntest du hier aggregateHourly/Daily/Monthly starten
}

// ---------------- Hilfsfunktionen ----------------

// Diese Funktionen könntest du z.B. für echte Aggregationen implementieren
/*
func aggregateHourly(db *sql.DB) { ... }
func aggregateDaily(db *sql.DB) { ... }
func aggregateMonthly(db *sql.DB) { ... }
*/
