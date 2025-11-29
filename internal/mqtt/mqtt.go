package mqtt

import (
	"database/sql"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/khorsmann/mqttlogger/internal/config"
)

// StartClient startet den MQTT-Client und registriert die Handler
func StartClient(cfg config.Config, db *sql.DB) mqtt.Client {
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker.Host).
		SetClientID(cfg.Broker.ClientID).
		SetUsername(cfg.Broker.Username).
		SetPassword(cfg.Broker.Password)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("MQTT Verbindung fehlgeschlagen: %v", token.Error())
	}

	// Subscription & Handler
	client.Subscribe(cfg.Topics.Wattwaechter, cfg.Broker.Qos, func(c mqtt.Client, m mqtt.Message) {
		handleWattwaechter(m.Topic(), string(m.Payload()), db, cfg)
	})

	client.Subscribe(cfg.Topics.Tasmota, cfg.Broker.Qos, func(c mqtt.Client, m mqtt.Message) {
		handleTasmota(m.Topic(), string(m.Payload()), db, cfg)
	})

	if cfg.Features.SolarEnabled {
		client.Subscribe("solar/#", cfg.Broker.Qos, func(c mqtt.Client, m mqtt.Message) {
			handleSolar(m.Topic(), string(m.Payload()), db, cfg)
		})
	}

	return client
}

// ---------------- Handler Stubs ----------------

func handleWattwaechter(topic, payload string, db *sql.DB, cfg config.Config) {
	log.Printf("[Wattwaechter] %s = %s", topic, payload)

	var msg struct {
		Time string `json:"Time"`
		E320 struct {
			EIn   float64 `json:"E_in"`
			EOut  float64 `json:"E_out"`
			Power int     `json:"Power"`
		} `json:"E320"`
	}

	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		log.Printf("[Wattwaechter] JSON Fehler: %v", err)
		return
	}

	// Zeit konvertieren
	t, err := time.Parse(time.RFC3339, msg.Time)
	if err != nil {
		log.Printf("[Wattwaechter] Zeitformatfehler: %v", err)
		t = time.Now()
	}

	stmt, err := db.Prepare(`
		INSERT INTO energy_data (timestamp_unix, timestamp_rfc3339, e_in, e_out, power)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		log.Printf("[Wattwaechter] DB Prepare Fehler: %v", err)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		t.Unix(),
		msg.Time,
		msg.E320.EIn,
		msg.E320.EOut,
		msg.E320.Power,
	)

	if err != nil {
		log.Printf("[Wattwaechter] DB Insert Fehler: %v", err)
	} else if cfg.Broker.SetDebug {
		log.Printf("[Wattwaechter] gespeichert E_in=%f E_out=%f Power=%d",
			msg.E320.EIn, msg.E320.EOut, msg.E320.Power)
	}
}
func handleTasmota(topic, payload string, db *sql.DB, cfg config.Config) {
	log.Printf("[Tasmota] %s = %s", topic, payload)

	var msg struct {
		Time   string `json:"Time"`
		Energy struct {
			Power float64 `json:"Power"`
		} `json:"ENERGY"`
	}

	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		log.Printf("[Tasmota] JSON Fehler: %v", err)
		return
	}

	t, err := time.Parse(time.RFC3339, msg.Time)
	if err != nil {
		log.Printf("[Tasmota] Zeitformatfehler: %v", err)
		t = time.Now()
	}

	stmt, err := db.Prepare(`
		INSERT INTO tasmota_data (device_id, timestamp_unix, timestamp_rfc3339, power)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		log.Printf("[Tasmota] DB Prepare Fehler: %v", err)
		return
	}
	defer stmt.Close()

	deviceID := strings.Split(topic, "/")[1]

	_, err = stmt.Exec(deviceID, t.Unix(), msg.Time, msg.Energy.Power)
	if err != nil {
		log.Printf("[Tasmota] DB Insert Fehler: %v", err)
	}
}

func handleSolar(topic, payload string, db *sql.DB, cfg config.Config) {
	loc, err := time.LoadLocation(cfg.Time.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	rfc3339Time := now.Format(time.RFC3339)
	unixTime := now.Unix()
	segments := strings.Split(topic, "/")

	if len(segments) < 2 {
		log.Printf("[Solar] Ungültiges Topic: %s", topic)
		return
	}

	deviceID := segments[1]
	channel := -1
	metric := strings.Join(segments[2:], "/")

	if val, err := strconv.ParseFloat(payload, 64); err == nil {
		stmt, _ := db.Prepare(`INSERT INTO solar_data (timestamp_unix, timestamp_rfc3339, device_id, channel, metric, value) VALUES (?, ?, ?, ?, ?, ?)`)
		defer stmt.Close()
		_, err = stmt.Exec(unixTime, rfc3339Time, deviceID, channel, metric, val)
		if err != nil {
			log.Printf("[Solar] DB Fehler solar_data: %v", err)
		} else if cfg.Broker.SetDebug {
			log.Printf("[Solar] %s/%d/%s = %f", deviceID, channel, metric, val)
		}
		return
	}

	stmt, _ := db.Prepare(`
		INSERT INTO solar_meta (device_id, channel, key, value)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(device_id, channel, key) DO UPDATE SET value = excluded.value
	`)
	defer stmt.Close()
	_, err = stmt.Exec(deviceID, channel, metric, payload)
	if err != nil {
		log.Printf("[Solar] DB Fehler solar_meta: %v", err)
	} else if cfg.Broker.SetDebug {
		log.Printf("[Solar] Meta gespeichert: %s/%d/%s = %s", deviceID, channel, metric, payload)
	}
}
