package mqtt

import (
	"database/sql"
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
	// TODO: Wattwächter-Logik implementieren
	log.Printf("[Wattwaechter] %s = %s", topic, payload)
}

func handleTasmota(topic, payload string, db *sql.DB, cfg config.Config) {
	// TODO: Tasmota-Logik implementieren
	log.Printf("[Tasmota] %s = %s", topic, payload)
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
