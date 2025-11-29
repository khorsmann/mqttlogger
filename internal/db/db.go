package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/khorsmann/mqttlogger/internal/config"

	_ "github.com/mattn/go-sqlite3"
)

// Open öffnet die SQLite DB
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	return db, nil
}

// InitDB erstellt Tabellen und startet Aggregation
func InitDB(db *sql.DB, cfg config.Config) error {
	if err := createTables(db); err != nil {
		return err
	}
	startAggregationLoop(db, cfg)
	return nil
}

// Tabellen erstellen
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
		// Persistente Aggregationen
		`CREATE TABLE IF NOT EXISTS daily_energy (
			day TEXT PRIMARY KEY,
			daily_consumption REAL
		);`,
		`CREATE TABLE IF NOT EXISTS weekly_energy (
			week TEXT PRIMARY KEY,
			weekly_consumption REAL
		);`,
		`CREATE TABLE IF NOT EXISTS monthly_energy_cost (
			month TEXT PRIMARY KEY,
			monthly_consumption REAL,
			monthly_cost REAL
		);`,
		`CREATE TABLE IF NOT EXISTS yearly_energy_cost_current (
			year INTEGER PRIMARY KEY,
			total_consumption REAL,
			total_cost REAL
		);`,
	}

	for _, t := range tables {
		if _, err := db.Exec(t); err != nil {
			return fmt.Errorf("Fehler beim Erstellen der Tabelle: %w", err)
		}
	}
	return nil
}

// Aggregationsfunktionen
func aggregateDaily(db *sql.DB) error {
	query := `
	INSERT OR REPLACE INTO daily_energy (day, daily_consumption)
	SELECT
		strftime('%Y-%m-%d', datetime(timestamp_unix, 'unixepoch')) AS day,
		MAX(e_in) - MIN(e_in) AS daily_consumption
	FROM energy_data
	GROUP BY day;
	`
	_, err := db.Exec(query)
	return err
}

func aggregateWeekly(db *sql.DB) error {
	query := `
	INSERT OR REPLACE INTO weekly_energy (week, weekly_consumption)
	SELECT
		strftime('%Y-%W', datetime(timestamp_unix, 'unixepoch')) AS week,
		MAX(e_in) - MIN(e_in) AS weekly_consumption
	FROM energy_data
	GROUP BY week;
	`
	_, err := db.Exec(query)
	return err
}

func aggregateMonthly(db *sql.DB, perKWh float64) error {
	query := `
	INSERT OR REPLACE INTO monthly_energy_cost (month, monthly_consumption, monthly_cost)
	SELECT
		strftime('%Y-%m', datetime(timestamp_unix, 'unixepoch')) AS month,
		MAX(e_in) - MIN(e_in) AS monthly_consumption,
		(MAX(e_in) - MIN(e_in)) * ? AS monthly_cost
	FROM energy_data
	GROUP BY month;
	`
	_, err := db.Exec(query, perKWh)
	return err
}

func aggregateYearly(db *sql.DB, perKWh float64) error {
	query := `
	INSERT OR REPLACE INTO yearly_energy_cost_current (year, total_consumption, total_cost)
	SELECT
		strftime('%Y', datetime(timestamp_unix, 'unixepoch')) AS year,
		MAX(e_in) - MIN(e_in) AS total_consumption,
		(MAX(e_in) - MIN(e_in)) * ? AS total_cost
	FROM energy_data
	GROUP BY year;
	`
	_, err := db.Exec(query, perKWh)
	return err
}

// Startet eine Hintergrund-Goroutine, die Aggregationen regelmäßig ausführt
func startAggregationLoop(db *sql.DB, cfg config.Config) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			if err := aggregateDaily(db); err != nil {
				log.Printf("Fehler tägliche Aggregation: %v", err)
			}
			if err := aggregateWeekly(db); err != nil {
				log.Printf("Fehler wöchentliche Aggregation: %v", err)
			}
			if err := aggregateMonthly(db, cfg.Cost.PerKWh); err != nil {
				log.Printf("Fehler monatliche Aggregation: %v", err)
			}
			if err := aggregateYearly(db, cfg.Cost.PerKWh); err != nil {
				log.Printf("Fehler jährliche Aggregation: %v", err)
			}

			<-ticker.C
		}
	}()
}
