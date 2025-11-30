package dbcmd

import (
	"fmt"

	"github.com/khorsmann/mqttlogger/internal/config"
	"github.com/spf13/cobra"
)

func NewPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Gibt den Pfad zur SQLite DB zurück",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, _ := config.Load("config.toml")
			fmt.Println(cfg.Database.Path)
		},
	}
}
