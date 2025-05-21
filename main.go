package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	Broker   BrokerConfig `toml:"broker"`
	Database string       `toml:"database"`
}

type BrokerConfig struct {
	Host     string `toml:"host"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	ClientID string `toml:"client_id"`
	Topic    string `toml:"topic"`
	Qos      byte   `toml:"qos"`
}

type EnergyData struct {
	Time        string  `json:"Time"`
	E_in        float64 `json:"E_in"`
	E_out       float64 `json:"E_out"`
	Power       int     `json:"Power"`
	MeterNumber string  `json:"Meter_Number"`
}

type SensorMessage struct {
	Sn struct {
		Time string     `json:"Time"`
		E320 EnergyData `json:"E320"`
	} `json:"sn"`
	Ver int `json:"ver"`
}

func main() {
	var config Config
	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		log.Fatalf("Fehler beim Laden der Config: %v", err)
	}

	db, err := sql.Open("sqlite3", config.Database)
	if err != nil {
		log.Fatalf("Fehler beim Öffnen der Datenbank: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS energy_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT,
			e_in REAL,
			e_out REAL,
			power INTEGER,
			meter_number TEXT,
			json TEXT
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
		log.Fatalf("Fehler beim Verbinden mit MQTT-Broker: %v", token.Error())
	}
	log.Println("Verbunden mit Broker")

	messageHandler := func(client mqtt.Client, msg mqtt.Message) {
		var sensorMsg SensorMessage
		err := json.Unmarshal(msg.Payload(), &sensorMsg)
		if err != nil {
			log.Printf("Fehler beim JSON-Unmarshal: %v", err)
			return
		}

		ed := sensorMsg.Sn.E320
		timestamp := sensorMsg.Sn.Time
		fmt.Printf("sn.E320.Meter_Number = %s\n", ed.MeterNumber)
		fmt.Printf("sn.E320.E_in = %.3f\n", ed.E_in)
		fmt.Printf("sn.E320.E_out = %.3f\n", ed.E_out)
		fmt.Printf("sn.E320.Power = %d\n", ed.Power)
		fmt.Printf("sn.Time = %s\n", timestamp)
		fmt.Printf("ver = %d\n", sensorMsg.Ver)

		stmt, err := db.Prepare("INSERT INTO energy_data (timestamp, e_in, e_out, power, meter_number, json) VALUES (?, ?, ?, ?, ?, ?)")
		if err != nil {
			log.Printf("Fehler beim Prepare: %v", err)
			return
		}
		defer stmt.Close()

		_, err = stmt.Exec(timestamp, ed.E_in, ed.E_out, ed.Power, ed.MeterNumber, string(msg.Payload()))
		if err != nil {
			log.Printf("Fehler beim Exec: %v", err)
			return
		}

		log.Printf("Gespeichert: %s - %.2f kWh - %d W\n", timestamp, ed.E_in, ed.Power)
	}

	token := client.Subscribe(config.Broker.Topic, config.Broker.Qos, messageHandler)
	if token.Wait() && token.Error() != nil {
		log.Fatalf("Fehler beim Subscribe: %v", token.Error())
	}

	log.Println("Läuft... (Strg+C zum Beenden)")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect(250)
	log.Println("Beendet")
}
