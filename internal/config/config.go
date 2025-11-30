package config

import (
	"github.com/BurntSushi/toml"
)

type BrokerConfig struct {
	Host     string `toml:"host"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	ClientID string `toml:"client_id"`
	Qos      byte   `toml:"qos"`
	SetDebug bool   `toml:"debug"`
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

type Config struct {
	Broker   BrokerConfig   `toml:"broker"`
	Database DatabaseConfig `toml:"database"`
	Time     TimeConfig     `toml:"time"`
	Topics   TopicsConfig   `toml:"topics"`
	Features FeatureFlags   `toml:"features"`
	Cost     CostConfig     `toml:"cost"`
}

func Load(path string) (Config, error) {
	var cfg Config
	_, err := toml.DecodeFile(path, &cfg)
	return cfg, err
}
