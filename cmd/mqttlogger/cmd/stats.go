package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/khorsmann/mqttlogger/internal/config"
	dbpkg "github.com/khorsmann/mqttlogger/internal/db"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statsCmd)
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Zeigt Statistiken zur Datenbank an",
	Run: func(cmd *cobra.Command, args []string) {

		cfg, err := config.Load("config.toml")
		if err != nil {
			fmt.Println("Config Fehler:", err)
			os.Exit(1)
		}

		db, err := dbpkg.Open(cfg.Database.Path)
		if err != nil {
			fmt.Println("DB Fehler:", err)
			os.Exit(1)
		}
		defer db.Close()

		var size int64
		if fi, err := os.Stat(cfg.Database.Path); err == nil {
			size = fi.Size()
		}

		color.Cyan("📦 Datenbank: %s", cfg.Database.Path)
		color.Cyan("📄 Größe: %.2f MB", float64(size)/1024/1024)

		tables := []string{
			"energy_data", "tasmota_data", "solar_data", "solar_meta",
			"daily_energy_raw", "weekly_energy_raw",
			"monthly_energy_cost_raw", "yearly_energy_cost_current_raw",
		}

		for _, t := range tables {
			var count int
			db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", t)).Scan(&count)
			color.Green("  %-30s %d rows", t, count)
		}
	},
}
