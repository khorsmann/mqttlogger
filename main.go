package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	"strings"

	"github.com/BurntSushi/toml"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	_ "github.com/mattn/go-sqlite3"
)

type TimeConfig struct {
	Timezone    string `toml:"timezone"`
	InputFormat string `toml:"input_format"`
}

type Config struct {
	Broker   BrokerConfig   `toml:"broker"`
	Database DatabaseConfig `toml:"database"`
	Time     TimeConfig     `toml:"time"`
	Topics   TopicsConfig   `toml:"topics"`
	Features FeatureFlags   `toml:"features"`
}

type BrokerConfig struct {
	Host     string `toml:"host"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	ClientID string `toml:"client_id"`
	Qos      byte   `toml:"qos"`
}

type TopicsConfig struct {
	Wattwaechter string `toml:"wattwaechter"`
	Tasmota      string `toml:"tasmota"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type FeatureFlags struct {
	TasmotaPowerEnabled bool `toml:"tasmota_power_enabled"`
}

type EnergyData struct {
	E_in        float64 `json:"E_in"`
	E_out       float64 `json:"E_out"`
	Power       int     `json:"Power"`
	MeterNumber string  `json:"Meter_Number"`
}

type SensorMessage struct {
	Time string     `json:"Time"`
	E320 EnergyData `json:"E320"`
}

type TasmotaENERGY struct {
	Power int `json:"Power"`
}

type TasmotaMessage struct {
	Time   string        `json:"Time"`
	ENERGY TasmotaENERGY `json:"ENERGY"`
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

	opts := mqtt.NewClientOptions().AddBroker(config.Broker.Host)
	opts.SetUsername(config.Broker.Username)
	opts.SetPassword(config.Broker.Password)
	opts.SetClientID(config.Broker.ClientID)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Fehler beim MQTT-Connect: %v", token.Error())
	}
	log.Println("Verbunden mit MQTT-Broker.")

	handleMessage := func(client mqtt.Client, msg mqtt.Message) {
		var base map[string]interface{}
		err := json.Unmarshal(msg.Payload(), &base)
		if err != nil {
			log.Printf("Ungültiges JSON: %v", err)
			return
		}

		topic := msg.Topic()
		timestampStr := base["Time"].(string)
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
			stmt, _ := db.Prepare(`INSERT INTO energy_data (timestamp_unix, timestamp_rfc3339, e_in, e_out, power) VALUES (?, ?, ?, ?, ?)`)
			defer stmt.Close()
			_, err = stmt.Exec(unixTime, rfc3339Time, sm.E320.E_in, sm.E320.E_out, sm.E320.Power)
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
				// device_id extrahieren z. B. aus "tele/tasmota_17A11C/SENSOR"
				segments := strings.Split(topic, "/")
				deviceID := ""
				if len(segments) >= 2 {
					deviceID = segments[1] // z.B. "tasmota_17A11C"
				}
				stmt, _ := db.Prepare(`INSERT INTO tasmota_data (device_id, timestamp_unix, timestamp_rfc3339, power) VALUES (?, ?, ?, ?)`)
				defer stmt.Close()
				_, err = stmt.Exec(deviceID, unixTime, rfc3339Time, tm.ENERGY.Power)
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

	log.Println("Läuft... (Strg+C zum Beenden)")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect(250)
	log.Println("Beendet.")
}
