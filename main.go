package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pelletier/go-toml/v2"
	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	Broker struct {
		Host     string `toml:"host"`
		Username string `toml:"username"`
		Password string `toml:"password"`
		Topic    string `toml:"topic"`
		ClientID string `toml:"client_id"`
	} `toml:"broker"`

	Database struct {
		Path string `toml:"path"`
	} `toml:"database"`
}

// Rekursive Funktion, um JSON-Map flach zu loggen
func logJsonMap(prefix string, data map[string]interface{}) {
	for k, v := range data {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]interface{}:
			logJsonMap(key, val)
		default:
			fmt.Printf("%s = %v\n", key, val)
		}
	}
}

func loadConfig(filename string) (Config, error) {
	var config Config
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, err
	}
	err = toml.Unmarshal(data, &config)
	return config, err
}

func main() {
	config, err := loadConfig("config.toml")
	if err != nil {
		log.Fatal("Konfigurationsfehler:", err)
	}

	db, err := sql.Open("sqlite3", config.Database.Path)
	if err != nil {
		log.Fatal(err)
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
		raw_json TEXT
	)`)
	if err != nil {
		log.Fatal(err)
	}

	opts := mqtt.NewClientOptions().
		AddBroker(config.Broker.Host).
		SetUsername(config.Broker.Username).
		SetPassword(config.Broker.Password).
		SetClientID(config.Broker.ClientID)

	opts.OnConnect = func(c mqtt.Client) {
		fmt.Println("Verbunden mit Broker")

		if token := c.Subscribe(config.Broker.Topic, 0, func(client mqtt.Client, msg mqtt.Message) {
			raw := string(msg.Payload())

			// JSON in map parsen
			var data map[string]interface{}
			if err := json.Unmarshal(msg.Payload(), &data); err != nil {
				log.Println("Fehler beim JSON-Parsing:", err)
				return
			}

			// Alle Keys/Values dynamisch loggen
			logJsonMap("", data)

			// Versuch die wichtigsten Werte manuell herauszuholen (optional)
			// Safety-Check: data["sn"] muss map sein
			var timestamp string
			var eIn, eOut float64
			var power int
			var meterNumber string

			if sn, ok := data["sn"].(map[string]interface{}); ok {
				if t, ok := sn["Time"].(string); ok {
					timestamp = t
				}
				if e320, ok := sn["E320"].(map[string]interface{}); ok {
					if einVal, ok := e320["E_in"].(float64); ok {
						eIn = einVal
					}
					if eoutVal, ok := e320["E_out"].(float64); ok {
						eOut = eoutVal
					}
					if powerVal, ok := e320["Power"].(float64); ok {
						power = int(powerVal)
					}
					if meterVal, ok := e320["Meter_Number"].(string); ok {
						meterNumber = meterVal
					}
				}
			}

			// DB-Insert vorbereiten
			stmt, err := db.Prepare(`
				INSERT INTO energy_data 
				(timestamp, e_in, e_out, power, meter_number, raw_json) 
				VALUES (?, ?, ?, ?, ?, ?)
			`)
			if err != nil {
				log.Println("DB Prepare Fehler:", err)
				return
			}
			defer stmt.Close()

			_, err = stmt.Exec(timestamp, eIn, eOut, power, meterNumber, raw)
			if err != nil {
				log.Println("DB Insert Fehler:", err)
				return
			}

			log.Printf("Gespeichert: %s - %.2f kWh - %d W\n", timestamp, eIn, power)
		}); token.Wait() && token.Error() != nil {
			log.Fatal(token.Error())
		}
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}

	fmt.Println("Läuft... (Strg+C zum Beenden)")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	client.Disconnect(250)
	fmt.Println("Beendet.")
}

