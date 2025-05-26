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
}

type BrokerConfig struct {
	Host     string `toml:"host"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	ClientID string `toml:"client_id"`
	Topic    string `toml:"topic"`
	Qos      byte   `toml:"qos"`
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

type SensorMessage struct {
	Time string     `json:"Time"`
	E320 EnergyData `json:"E320"`
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
        )
    `)
	if err != nil {
		log.Fatalf("Fehler beim Erstellen der Tabelle: %v", err)
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

	messageHandler := func(client mqtt.Client, msg mqtt.Message) {
		var sensorMsg SensorMessage
		err := json.Unmarshal(msg.Payload(), &sensorMsg)
		if err != nil {
			log.Printf("Fehler beim JSON-Unmarshal: %v", err)
			return
		}

		ed := sensorMsg.E320
		timestampStr := sensorMsg.Time

		loc, err := time.LoadLocation(config.Time.Timezone)
		if err != nil {
			log.Printf("Fehler beim Laden der Zeitzone: %v", err)
			return
		}

		localTime, err := time.ParseInLocation(config.Time.InputFormat, timestampStr, loc)
		if err != nil {
			log.Printf("Fehler beim Parsen der Zeit: %v", err)
			return
		}

		utcTime := localTime.UTC()
		unixTime := utcTime.Unix()
		rfc3339Time := localTime.Format(time.RFC3339)

		stmt, err := db.Prepare(`
            INSERT INTO energy_data (timestamp_unix, timestamp_rfc3339, e_in, e_out, power)
            VALUES (?, ?, ?, ?, ?)
        `)
		if err != nil {
			log.Printf("Fehler beim Prepare: %v", err)
			return
		}
		defer stmt.Close()

		_, err = stmt.Exec(unixTime, rfc3339Time, ed.E_in, ed.E_out, ed.Power)
		if err != nil {
			log.Printf("Fehler beim Exec: %v", err)
			return
		}

		log.Printf("Gespeichert: %.2f kWh, %d W @ %s (%d)", ed.E_in, ed.Power, rfc3339Time, unixTime)
	}

	if token := client.Subscribe(config.Broker.Topic, config.Broker.Qos, messageHandler); token.Wait() && token.Error() != nil {
		log.Fatalf("Fehler beim Subscribe: %v", token.Error())
	}
	log.Printf("Abonniert auf Topic: %s", config.Broker.Topic)

	log.Println("Läuft... (Strg+C zum Beenden)")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect(250)
	log.Println("Beendet.")
}
