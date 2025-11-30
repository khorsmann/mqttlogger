package db

import (
	"fmt"
	"io"
	"os"

	"database/sql"
)

// CreateBackup erzeugt eine saubere Einzeldatei-DB ohne WAL & SHM
func CreateBackup(db *sql.DB, sourceDBPath, backupPath string) error {

	// WAL flushen
	if _, err := db.Exec("PRAGMA wal_checkpoint(FULL);"); err != nil {
		return fmt.Errorf("wal checkpoint fehlgeschlagen: %w", err)
	}

	// Temporär WAL deaktivieren → Einzeldatei garantieren
	if _, err := db.Exec("PRAGMA journal_mode=DELETE;"); err != nil {
		return fmt.Errorf("wal-abschaltung fehlgeschlagen: %w", err)
	}

	// Datei kopieren
	if err := copyFile(sourceDBPath, backupPath); err != nil {
		return fmt.Errorf("backup fehlgeschlagen: %w", err)
	}

	// WAL wieder aktivieren
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")

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

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}
