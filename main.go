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

type BrokerConfig struct {
	Host     string `toml:"host"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	ClientID string `toml:"client_id"`
	Qos      byte   `toml:"qos"`
}

type Config struct {
	Broker   BrokerConfig   `toml:"broker"`
	Database DatabaseConfig `toml:"database"`
	Time     TimeConfig     `toml:"time"`
	Topics   TopicsConfig   `toml:"topics"`
	Features FeatureFlags   `toml:"features"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type EnergyData struct {
	E_in        float64 `json:"E_in"`
	E_out       float64 `json:"E_out"`
	Power       int     `json:"Power"`
	MeterNumber string  `json:"Meter_Number"`
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

func startDBAggregation(db *sql.DB, hour, minute int) {
	// Automatischer Checkpoint ab ~1 MB WAL
	_, _ = db.Exec("PRAGMA wal_autocheckpoint=1000;")

	// Channel für manuelle Trigger (USR1)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGUSR1)

	// Aggregationsfunktion
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
			`, table, strings.ReplaceAll(otherCols, ",", ", "), valueColumn, otherCols, valueColumn, tmpTable)

			if _, err := db.Exec(insert); err != nil {
				log.Printf("Fehler beim Einfügen aggregierter Daten in %s: %v", table, err)
				return
			}

			_, _ = db.Exec(fmt.Sprintf(`DROP TABLE %s`, tmpTable))
			log.Printf("Aggregation in %s abgeschlossen", table)
		}

		// Tabellen aggregieren
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
	}

	// Nachtjob
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

	// Manueller Trigger per Signal
	go func() {
		for range sigChan {
			log.Println("Signal USR1 empfangen – Aggregation wird gestartet...")
			runAggregation()
		}
	}()
}

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

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS energy_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			e_in REAL,
			e_out REAL,
			power INTEGER
		)`)
	if err != nil {
		log.Fatalf("Fehler beim Erstellen der Tabelle energy_data: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tasmota_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			power INTEGER
		)`)
	if err != nil {
		log.Fatalf("Fehler beim Erstellen der Tabelle tasmota_data: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS solar_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp_unix INTEGER,
			timestamp_rfc3339 TEXT,
			device_id TEXT,
			channel INTEGER,
			metric TEXT,
			value REAL
		)`)
	if err != nil {
		log.Fatalf("Fehler beim Erstellen der Tabelle solar_data: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS solar_meta (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT,
			channel INTEGER,
			key TEXT,
			value TEXT,
			UNIQUE(device_id, channel, key)
		)`)
	if err != nil {
		log.Fatalf("Fehler beim Erstellen der Tabelle solar_meta: %v", err)
	}

	stmtWatt, _ := db.Prepare(`INSERT INTO energy_data (timestamp_unix, timestamp_rfc3339, e_in, e_out, power) VALUES (?, ?, ?, ?, ?)`)
	defer stmtWatt.Close()
	stmtTasmota, _ := db.Prepare(`INSERT INTO tasmota_data (device_id, timestamp_unix, timestamp_rfc3339, power) VALUES (?, ?, ?, ?)`)
	defer stmtTasmota.Close()

	startDBAggregation(db, 3, 0) // Täglich um 03:00 Uhr

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
			log.Printf("Wattwächter: %d W @ %s", sm.E320.Power, rfc3339Time)

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
				log.Printf("Tasmota: %s %d W @ %s", deviceID, tm.ENERGY.Power, rfc3339Time)
			}
		}
	}

	client.Subscribe(config.Topics.Wattwaechter, config.Broker.Qos, handleMessage)
	log.Printf("Abonniert auf Topic: %s", config.Topics.Wattwaechter)

	if config.Features.TasmotaPowerEnabled {
		client.Subscribe(config.Topics.Tasmota, config.Broker.Qos, handleMessage)
		log.Printf("Abonniert auf Topic: %s", config.Topics.Tasmota)
	}

	if config.Features.SolarEnabled {
		client.Subscribe("solar/#", config.Broker.Qos, handleMessage)
		log.Printf("Abonniert auf Topic: solar/#")
	}

	log.Println("Läuft... (Strg+C zum Beenden)")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect(250)
	log.Println("Beendet.")
}

func handleSolar(topic string, payload string, db *sql.DB, config Config) {
	loc, _ := time.LoadLocation(config.Time.Timezone)
	now := time.Now().In(loc)
	rfc3339Time := now.Format(time.RFC3339)
	unixTime := now.UTC().Unix()

	segments := strings.Split(topic, "/")
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
			log.Printf("Ungültiges Solar-Topic: %s", topic)
			return
		}
		deviceID = segments[1]
		ch, err := strconv.Atoi(segments[2])
		if err != nil {
			log.Printf("Ungültiger Channel: %s", segments[2])

			return
		}
		channel = ch
		metric = strings.Join(segments[3:], "/")
	}

	// Versuche Wert als float
	if val, err := strconv.ParseFloat(payload, 64); err == nil {
		stmt, _ := db.Prepare(`INSERT INTO solar_data (timestamp_unix, timestamp_rfc3339, device_id, channel, metric, value) VALUES (?, ?, ?, ?, ?, ?)`)
		defer stmt.Close()
		_, err = stmt.Exec(unixTime, rfc3339Time, deviceID, channel, metric, val)
		if err != nil {
			log.Printf("Fehler DB solar_data: %v", err)

		} else {
			log.Printf("Solar: %s/%d/%s = %f @ %s", deviceID, channel, metric, val, rfc3339Time)
		}
		return
	}

	// Andernfalls Textwert → speichere in solar_meta
	stmt, _ := db.Prepare(`
		INSERT INTO solar_meta (device_id, channel, key, value)
		VALUES (?, ?, ?, ?)

		ON CONFLICT(device_id, channel, key) DO UPDATE SET value = excluded.value
	`)
	defer stmt.Close()
	_, err := stmt.Exec(deviceID, channel, metric, payload)
	if err != nil {
		log.Printf("Fehler DB solar_meta: %v", err)
	} else {
		log.Printf("Solar-Meta gespeichert: %s/%d/%s = %s", deviceID, channel, metric, payload)
	}
}
