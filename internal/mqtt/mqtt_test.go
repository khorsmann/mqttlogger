package mqtt

import (
	"database/sql"
	"testing"

	"github.com/khorsmann/mqttlogger/internal/config"
	_ "github.com/mattn/go-sqlite3"
)

// create only the tables the handlers touch to keep the tests lightweight.
func newMQTTTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	schema := []string{
		`CREATE TABLE energy_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			e_in REAL,
			e_out REAL,
			power INTEGER
		);`,
		`CREATE TABLE tasmota_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			power INTEGER
		);`,
	}
	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return db
}

func TestHandleWattwaechterPersistsReading(t *testing.T) {
	db := newMQTTTestDB(t)
	defer db.Close()

	cfg := config.Config{
		Time: config.TimeConfig{Timezone: "Europe/Berlin"},
	}

	payload := `{"Time":"2025-11-24T20:00:00+01:00","E320":{"E_in":123.4,"E_out":1.2,"Power":456,"Meter_Number":"abc"}}`

	handleWattwaechter("tele/WattWaechter_2E6BD4/SENSOR", payload, db, cfg)

	var count int
	var eIn, eOut float64
	var power float64
	err := db.QueryRow(`SELECT COUNT(*), e_in, e_out, power FROM energy_data`).Scan(&count, &eIn, &eOut, &power)
	if err != nil {
		t.Fatalf("select energy_data: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
	if eIn != 123.4 || eOut != 1.2 || power != 456 {
		t.Fatalf("unexpected values eIn=%v eOut=%v power=%v", eIn, eOut, power)
	}
}

func TestHandleTasmotaPersistsPower(t *testing.T) {
	db := newMQTTTestDB(t)
	defer db.Close()

	cfg := config.Config{
		Time: config.TimeConfig{Timezone: "Europe/Berlin"},
	}

	payload := `{"Time":"2025-11-24T19:00:00Z","ENERGY":{"Power":42}}`

	handleTasmota("tele/device123/SENSOR", payload, db, cfg)

	var count int
	var deviceID string
	var power float64
	err := db.QueryRow(`SELECT COUNT(*), device_id, power FROM tasmota_data`).Scan(&count, &deviceID, &power)
	if err != nil {
		t.Fatalf("select tasmota_data: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
	if deviceID != "device123" || power != 42 {
		t.Fatalf("unexpected values deviceID=%s power=%v", deviceID, power)
	}
}
