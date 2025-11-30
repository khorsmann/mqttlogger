package db

import (
	"os"

	"github.com/khorsmann/mqttlogger/internal/cli"
)

func RestoreBackup(dbPath, backupPath string, verbose, debug bool) error {

	if verbose {
		cli.Info("Entferne alte WAL/SHM Dateien…")
	}

	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")
	os.Remove(dbPath)

	for i := 0; i <= 100; i += 5 {
		cli.ProgressBar(i)
	}

	if verbose {
		cli.Info("Kopiere neue DB…")
	}

	if err := copyFile(backupPath, dbPath); err != nil {
		return err
	}

	if verbose {
		cli.Info("Reaktiviere WAL-Modus…")
	}

	db, err := Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	_, _ = db.Exec("PRAGMA journal_mode=WAL;")

	return nil
}
