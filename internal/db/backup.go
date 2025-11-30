package db

import (
	"database/sql"
	"fmt"
	"io"
	"os"

	"github.com/khorsmann/mqttlogger/internal/cli"
)

func CreateBackup(db *sql.DB, sourceDBPath, backupPath string, verbose, debug bool) error {

	if verbose {
		cli.Info("Führe WAL Checkpoint durch…")
	}
	if debug {
		cli.Info("PRAGMA wal_checkpoint(FULL)")
	}

	if _, err := db.Exec("PRAGMA wal_checkpoint(FULL);"); err != nil {
		return fmt.Errorf("wal checkpoint fehlgeschlagen: %w", err)
	}

	if verbose {
		cli.Info("Deaktiviere WAL-Modus…")
	}

	if _, err := db.Exec("PRAGMA journal_mode=DELETE;"); err != nil {
		return fmt.Errorf("WAL-Abschaltung fehlgeschlagen: %w", err)
	}

	// DB-Größe anzeigen
	if fi, err := os.Stat(sourceDBPath); err == nil && verbose {
		cli.Info(fmt.Sprintf("Aktuelle DB-Größe: %.2f MB", float64(fi.Size())/1024/1024))
	}

	// Fortschrittsbalken
	for i := 0; i <= 100; i += 4 {
		cli.ProgressBar(i)
	}

	if verbose {
		cli.Info("Kopiere Datei…")
	}
	if err := copyFile(sourceDBPath, backupPath); err != nil {
		return err
	}

	_, _ = db.Exec("PRAGMA journal_mode=WAL;")

	if verbose {
		cli.Info("Backup abgeschlossen.")
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
