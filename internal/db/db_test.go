package db

import (
	"database/sql"
	"testing"
	"time"

	"github.com/khorsmann/mqttlogger/internal/config"
)

// helper to build a fresh in-memory database with schema
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := createTables(db); err != nil {
		t.Fatalf("createTables: %v", err)
	}
	if err := createViews(db); err != nil {
		t.Fatalf("createViews: %v", err)
	}
	return db
}

func TestAggregateIgnoresOldTimestampsAndReplacesValues(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Insert readings: one invalid (old epoch), two valid days across two months.
	insert := `
		INSERT INTO energy_data (timestamp_unix, timestamp_rfc3339, e_in, e_out, power)
		VALUES (?, ?, ?, ?, ?);
	`
	rows := []struct {
		ts  time.Time
		eIn float64
		pwr int
	}{
		{time.Unix(-3600, 0), 10, 0},                            // should be ignored
		{time.Date(2025, 11, 1, 8, 0, 0, 0, time.UTC), 100, 0},  // start of Nov
		{time.Date(2025, 11, 1, 20, 0, 0, 0, time.UTC), 105, 0}, // end of same day -> 5 kWh
		{time.Date(2025, 12, 2, 9, 0, 0, 0, time.UTC), 200, 0},  // start of Dec
		{time.Date(2025, 12, 2, 21, 0, 0, 0, time.UTC), 210, 0}, // end of same day -> 10 kWh
	}
	for _, r := range rows {
		if _, err := db.Exec(insert, r.ts.Unix(), r.ts.Format(time.RFC3339), r.eIn, 0, r.pwr); err != nil {
			t.Fatalf("insert energy_data: %v", err)
		}
	}

	if err := aggregateDaily(db); err != nil {
		t.Fatalf("aggregateDaily: %v", err)
	}
	if err := aggregateMonthly(db, 1.0); err != nil { // cost = consumption for easy asserts
		t.Fatalf("aggregateMonthly: %v", err)
	}
	if err := aggregateYearly(db, 1.0); err != nil {
		t.Fatalf("aggregateYearly: %v", err)
	}

	// daily: only two real days, correct consumption
	type kv struct {
		key string
		val float64
	}
	checkTable := func(table, keyCol, valCol string, want map[string]float64) {
		rows, err := db.Query("SELECT " + keyCol + ", " + valCol + " FROM " + table)
		if err != nil {
			t.Fatalf("select %s: %v", table, err)
		}
		defer rows.Close()
		got := map[string]float64{}
		for rows.Next() {
			var k string
			var v float64
			if err := rows.Scan(&k, &v); err != nil {
				t.Fatalf("scan %s: %v", table, err)
			}
			got[k] = v
		}
		if len(got) != len(want) {
			t.Fatalf("%s length mismatch: got %d, want %d", table, len(got), len(want))
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("%s mismatch for %s: got %v, want %v", table, k, got[k], v)
			}
		}
	}

	checkTable("daily_energy_raw", "day", "daily_consumption", map[string]float64{
		"2025-11-01": 5,
		"2025-12-02": 10,
	})

	checkTable("monthly_energy_cost_raw", "month", "consumption", map[string]float64{
		"2025-11": 100, // consumption is measured between month boundaries
		"2025-12": 10,
	})

	checkTable("yearly_energy_cost_current_raw", "year", "consumption", map[string]float64{
		"2025": 110, // max-min over the whole year (210 - 100)
	})
}

func TestInitDBCreatesSchema(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer db.Close()

	cfg := config.Config{
		Cost: config.CostConfig{PerKWh: 0.3},
	}

	if err := InitDB(db, cfg); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	tables := []string{
		"energy_data", "tasmota_data", "solar_data", "solar_meta",
		"daily_energy_raw", "weekly_energy_raw",
		"monthly_energy_cost_raw", "yearly_energy_cost_current_raw",
	}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}

	views := []string{"daily_energy", "weekly_energy", "monthly_energy_cost", "yearly_energy_cost_current"}
	for _, view := range views {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='view' AND name=?`, view).Scan(&name)
		if err != nil {
			t.Fatalf("view %s missing: %v", view, err)
		}
	}
}
