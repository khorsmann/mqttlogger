package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/BurntSushi/toml"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// ---------------- Config ----------------

type BrokerConfig struct {
	Host     string `toml:"host"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	ClientID string `toml:"client_id"`
	Qos      byte   `toml:"qos"`
	SetDebug bool   `toml:"debug"`
}

type Config struct {
	Broker   BrokerConfig   `toml:"broker"`
	Database DatabaseConfig `toml:"database"`
	Time     TimeConfig     `toml:"time"`
	Topics   TopicsConfig   `toml:"topics"`
	Features FeatureFlags   `toml:"features"`
	Cost     CostConfig     `toml:"cost"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type CostConfig struct {
	PerKWh float64 `toml:"per_kwh"`
}

type FeatureFlags struct {
	TasmotaPowerEnabled bool `toml:"tasmota_power"`
	SolarEnabled        bool `toml:"solar"`
}

type TimeConfig struct {
	Timezone    string `toml:"timezone"`
	InputFormat string `toml:"input_format"`
}

type TopicsConfig struct {
	Wattwaechter string `toml:"wattwaechter"`
	Tasmota      string `toml:"tasmota"`
}

// ---------------- MQTT & Datenstrukturen ----------------

type EnergyData struct {
	E_in        float64 `json:"E_in"`
	E_out       float64 `json:"E_out"`
	Power       int     `json:"Power"`
	MeterNumber string  `json:"Meter_Number"`
}

type TasmotaMessage struct {
	Time   string `json:"Time"`
	ENERGY struct {
		Power int `json:"Power"`
	} `json:"ENERGY"`
}

type SensorMessage struct {
	Time string `json:"Time"`
	E320 struct {
		E_in  float64 `json:"E_in"`
		E_out float64 `json:"E_out"`
		Power int     `json:"Power"`
	} `json:"E320"`
}

// ---------------- Views erstellen ----------------

func createViews(db *sql.DB, costPerKWh float64) error {
	views := []string{
		fmt.Sprintf(`
			CREATE VIEW IF NOT EXISTS monthly_energy_cost AS
			SELECT
				strftime('%%Y-%%m', datetime(timestamp_unix, 'unixepoch')) AS month,
				MAX(e_in) - MIN(e_in) AS monthly_consumption,
				ROUND((MAX(e_in) - MIN(e_in)) * %f, 2) AS monthly_cost
			FROM energy_data
			WHERE timestamp_unix >= strftime('%%s', 'now', 'start of month', '-11 months')
			GROUP BY month
			ORDER BY month DESC
			LIMIT 12
		`, costPerKWh),

		`CREATE VIEW IF NOT EXISTS monthly_energy_cost_total AS
		 SELECT
			 SUM(monthly_consumption) AS total_consumption,
			 SUM(monthly_cost) AS total_cost
		 FROM monthly_energy_cost`,

		fmt.Sprintf(`
			CREATE VIEW IF NOT EXISTS yearly_energy_cost_current AS
			SELECT
				SUM(monthly_consumption) AS total_consumption,
				ROUND(SUM(monthly_cost), 2) AS total_cost
			FROM (
				SELECT
					strftime('%%Y-%%m', datetime(timestamp_unix, 'unixepoch')) AS month,
					MAX(e_in) - MIN(e_in) AS monthly_consumption,
					(MAX(e_in) - MIN(e_in)) * %f AS monthly_cost
				FROM energy_data
				WHERE substr(strftime('%%Y-%%m', datetime(timestamp_unix, 'unixepoch')), 1, 4) = strftime('%%Y', 'now')
				GROUP BY month
			)
		`, costPerKWh),
	}

	for _, v := range views {
		if _, err := db.Exec(v); err != nil {
			return fmt.Errorf("Fehler beim Erstellen von View: %v", err)
		}
	}

	return nil
}

// ---------------- DB Aggregation ----------------

func startDBAggregation(db *sql.DB, hour, minute int, costPerKWh float64) {
	_, _ = db.Exec("PRAGMA wal_autocheckpoint=1000;")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGUSR1)

	runAggregation := func() {
		log.Println("Starte DB-Aggregation...")

		aggregate := func(table string, valueColumn string, otherCols string) {
			tmpTable := table + "_agg_tmp"
			createTmp := fmt.Sprintf(`
				CREATE TEMP TABLE %s AS
				SELECT
					CAST(strftime('%%s', strftime('%%Y-%%m-%%d %%H:00:00', timestamp_unix, 'unixepoch')) AS INTEGER) AS timestamp_unix,
					strftime('%%Y-%%m-%%dT%%H:00:00', timestamp_unix, 'unixepoch') AS timestamp_rfc3339,
					%s,
					AVG(%s) AS %s
				FROM %s
				WHERE timestamp_unix < strftime('%%s', 'now', '-30 days')
				GROUP BY %s, strftime('%%Y-%%m-%%d %%H', timestamp_unix, 'unixepoch');
			`, tmpTable, otherCols, valueColumn, valueColumn, table, otherCols)

			if _, err := db.Exec(createTmp); err != nil {
				log.Printf("Fehler beim Erstellen temporärer Tabelle für %s: %v", table, err)
				return
			}

			if _, err := db.Exec(
				fmt.Sprintf(`DELETE FROM %s WHERE timestamp_unix < strftime('%%s', 'now', '-30 days')`, table),
			); err != nil {
				log.Printf("Fehler beim Löschen alter Daten in %s: %v", table, err)
				return
			}

			insert := fmt.Sprintf(`
				INSERT INTO %s (timestamp_unix, timestamp_rfc3339, %s, %s)
				SELECT timestamp_unix, timestamp_rfc3339, %s, %s FROM %s
			`, table, strings.ReplaceAll(otherCols, ",", ", "), valueColumn, valueColumn, otherCols, tmpTable)

			if _, err := db.Exec(insert); err != nil {
				log.Printf("Fehler beim Einfügen aggregierter Daten in %s: %v", table, err)
				return
			}

			_, _ = db.Exec(fmt.Sprintf(`DROP TABLE %s`, tmpTable))
			log.Printf("Aggregation in %s abgeschlossen", table)
		}

		aggregate("energy_data", "power", "e_in, e_out")
		aggregate("tasmota_data", "power", "device_id")
		aggregate("solar_data", "value", "device_id, channel, metric")

		// WAL-Checkpoint
		if _, err := db.Exec("PRAGMA wal_checkpoint(FULL);"); err != nil {
			log.Printf("Fehler beim WAL-Checkpoint: %v", err)
		} else {
			log.Println("WAL-Checkpoint durchgeführt – WAL-Datei geleert")
		}

		// VACUUM
		if _, err := db.Exec("VACUUM"); err != nil {
			log.Printf("Fehler beim VACUUM: %v", err)
		} else {
			log.Println("VACUUM abgeschlossen – DB verschlankt")
		}

		// Views nach Aggregation aktualisieren
		if err := createViews(db, costPerKWh); err != nil {
			log.Printf("Fehler beim Aktualisieren der Views: %v", err)
		} else {
			log.Println("Views aktualisiert")
		}
	}

	go func() {
		for {
			now := time.Now()
			nextRun := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
			if now.After(nextRun) {
				nextRun = nextRun.Add(24 * time.Hour)
			}
			time.Sleep(time.Until(nextRun))
			runAggregation()
		}
	}()

	go func() {
		for range sigChan {
			log.Println("Signal USR1 empfangen – Aggregation wird gestartet...")
			runAggregation()
		}
	}()
}

// ---------------- Solar Handler ----------------

func handleSolar(topic string, payload string, db *sql.DB, config Config) {
	loc, _ := time.LoadLocation(config.Time.Timezone)
	now := time.Now().In(loc)
	rfc3339Time := now.Format(time.RFC3339)
	unixTime := now.UTC().Unix()
	segments := strings.Split(topic, "/")
	debug := config.Broker.SetDebug
	if len(segments) < 2 {
		log.Printf("Ungültiges Solar-Topic: %s", topic)
		return
	}

	var deviceID string
	var channel int
	var metric string

	switch segments[1] {
	case "ac", "dc":
		deviceID = segments[1]
		channel = -1
		metric = strings.Join(segments[2:], "/")
	case "dtu":
		return
	case "today_energy_sum":
		deviceID = "summary"
		channel = -1
		metric = "today_energy_sum"
	default:
		if len(segments) < 4 {
			if debug {
				log.Printf("Ungültiges Solar-Topic: %s", topic)
			}
			return
		}
		deviceID = segments[1]
		ch, err := strconv.Atoi(segments[2])
		if err != nil {
			if debug {
				log.Printf("Ungültiger Channel: %s", segments[2])
			}
			return
		}
		channel = ch
		metric = strings.Join(segments[3:], "/")
	}

	if val, err := strconv.ParseFloat(payload, 64); err == nil {
		stmt, _ := db.Prepare(`INSERT INTO solar_data (timestamp_unix, timestamp_rfc3339, device_id, channel, metric, value) VALUES (?, ?, ?, ?, ?, ?)`)
		defer stmt.Close()
		_, err = stmt.Exec(unixTime, rfc3339Time, deviceID, channel, metric, val)
		if err != nil {
			log.Printf("Fehler DB solar_data: %v", err)
		} else if debug {
			log.Printf("Solar: %s/%d/%s = %f @ %s", deviceID, channel, metric, val, rfc3339Time)
		}
		return
	}

	stmt, _ := db.Prepare(`
		INSERT INTO solar_meta (device_id, channel, key, value)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(device_id, channel, key) DO UPDATE SET value = excluded.value
	`)
	defer stmt.Close()
	_, err := stmt.Exec(deviceID, channel, metric, payload)
	if err != nil {
		log.Printf("Fehler DB solar_meta: %v", err)
	} else if debug {
		log.Printf("Solar-Meta gespeichert: %s/%d/%s = %s", deviceID, channel, metric, payload)
	}
}

// ---------------- Main ----------------

func main() {
	var config Config
	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		log.Fatalf("Fehler beim Laden der Config: %v", err)
	}

	dbPath, _ := filepath.Abs(config.Database.Path)
	log.Printf("Datenbank-Dateipfad: %s", dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Fehler beim Öffnen der Datenbank: %v", err)
	}
	defer db.Close()

	_, _ = db.Exec("PRAGMA journal_mode=WAL;")

	// Tabellen erstellen
	tables := []string{
		`CREATE TABLE IF NOT EXISTS energy_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			e_in REAL,
			e_out REAL,
			power INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS tasmota_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			power INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS solar_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			device_id TEXT,
			channel INTEGER,
			metric TEXT,
			value REAL
		)`,
		`CREATE TABLE IF NOT EXISTS solar_meta (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT,
			channel INTEGER,
			key TEXT,
			value TEXT,
			UNIQUE(device_id, channel, key)
		)`,
	}

	for _, t := range tables {
		if _, err := db.Exec(t); err != nil {
			log.Fatalf("Fehler beim Erstellen der Tabelle: %v", err)
		}
	}

	// Views erstellen
	if err := createViews(db, config.Cost.PerKWh); err != nil {
		log.Fatalf("Fehler beim Erstellen der Views: %v", err)
	}

	// Aggregation starten (täglich 03:00 Uhr)
	startDBAggregation(db, 3, 0, config.Cost.PerKWh)

	// Prepare Statements
	stmtWatt, _ := db.Prepare(`INSERT INTO energy_data (timestamp_unix, timestamp_rfc3339, e_in, e_out, power) VALUES (?, ?, ?, ?, ?)`)
	defer stmtWatt.Close()
	stmtTasmota, _ := db.Prepare(`INSERT INTO tasmota_data (device_id, timestamp_unix, timestamp_rfc3339, power) VALUES (?, ?, ?, ?)`)
	defer stmtTasmota.Close()

	// --- MQTT Setup ---
	opts := mqtt.NewClientOptions().AddBroker(config.Broker.Host)
	opts.SetUsername(config.Broker.Username)
	opts.SetPassword(config.Broker.Password)
	opts.SetClientID(config.Broker.ClientID)
	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		log.Printf("Verbindung verloren: %v", err)
	}
	opts.OnConnect = func(c mqtt.Client) {
		log.Println("Erneut mit MQTT-Broker verbunden.")
	}

	debug := config.Broker.SetDebug
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Fehler beim MQTT-Connect: %v", token.Error())
	}
	log.Println("Verbunden mit MQTT-Broker.")

	handleMessage := func(client mqtt.Client, msg mqtt.Message) {
		topic := msg.Topic()

		if strings.HasPrefix(topic, "solar/") && config.Features.SolarEnabled {
			handleSolar(topic, string(msg.Payload()), db, config)
			return
		}

		var base map[string]interface{}
		err := json.Unmarshal(msg.Payload(), &base)
		if err != nil {
			log.Printf("Ungültiges JSON: %v", err)
			return
		}

		timestampStr, ok := base["Time"].(string)
		if !ok {
			log.Printf("Zeitfeld fehlt oder ist ungültig im JSON: %v", base)
			return
		}
		loc, _ := time.LoadLocation(config.Time.Timezone)
		localTime, err := time.ParseInLocation(config.Time.InputFormat, timestampStr, loc)
		if err != nil {
			log.Printf("Zeitfehler: %v", err)
			return
		}
		utcTime := localTime.UTC()
		rfc3339Time := localTime.Format(time.RFC3339)
		unixTime := utcTime.Unix()

		switch topic {
		case config.Topics.Wattwaechter:
			var sm SensorMessage
			if err := json.Unmarshal(msg.Payload(), &sm); err != nil {
				log.Printf("Fehler beim Parsen Wattwächter: %v", err)
				return
			}
			_, err = stmtWatt.Exec(unixTime, rfc3339Time, sm.E320.E_in, sm.E320.E_out, sm.E320.Power)
			if err != nil {
				log.Printf("Fehler DB Wattwächter: %v", err)
			}
			if debug {
				log.Printf("Wattwächter: %d W @ %s", sm.E320.Power, rfc3339Time)
			}

		case config.Topics.Tasmota:
			if config.Features.TasmotaPowerEnabled {
				var tm TasmotaMessage
				if err := json.Unmarshal(msg.Payload(), &tm); err != nil {
					log.Printf("Fehler beim Parsen Tasmota: %v", err)
					return
				}
				segments := strings.Split(topic, "/")
				deviceID := ""
				if len(segments) >= 2 {
					deviceID = segments[1]
				}
				_, err = stmtTasmota.Exec(deviceID, unixTime, rfc3339Time, tm.ENERGY.Power)
				if err != nil {
					log.Printf("Fehler DB Tasmota: %v", err)
				}
				if debug {
					log.Printf("Tasmota: %s %d W @ %s", deviceID, tm.ENERGY.Power, rfc3339Time)
				}
			}
		}
	}

	// Subscribe Topics
	client.Subscribe(config.Topics.Wattwaechter, config.Broker.Qos, handleMessage)
	if config.Features.TasmotaPowerEnabled {
		client.Subscribe(config.Topics.Tasmota, config.Broker.Qos, handleMessage)
	}
	if config.Features.SolarEnabled {
		client.Subscribe("solar/#", config.Broker.Qos, handleMessage)
	}

	log.Println("Läuft... (Strg+C zum Beenden)")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect(250)
	log.Println("Beendet.")
}
