package db

import (
	"fmt"
	"os"
)

// RestoreBackup ersetzt die DB-Datei sicher
func RestoreBackup(dbPath, backupPath string) error {

	// 1. WAL / SHM entfernen
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")

	// 2. bestehende DB löschen
	os.Remove(dbPath)

	// 3. Backup kopieren
	if err := copyFile(backupPath, dbPath); err != nil {
		return fmt.Errorf("restore fehlgeschlagen: %w", err)
	}

	// 4. WAL wieder aktivieren
	db, err := Open(dbPath)
	if err != nil {
		return fmt.Errorf("fehler beim reopen: %w", err)
	}
	defer db.Close()

	_, _ = db.Exec("PRAGMA journal_mode=WAL;")

	return nil
}
