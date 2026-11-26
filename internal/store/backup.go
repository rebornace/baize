package store

import "errors"

// ErrBackupUnsupported means the active driver cannot produce an in-app backup.
var ErrBackupUnsupported = errors.New("backup is not supported for this store driver")

// BackupTo is unsupported for the in-memory driver.
func (s *Memory) BackupTo(path string) error {
	return ErrBackupUnsupported
}

// BackupTo writes a consistent SQLite snapshot with VACUUM INTO. Postgres
// deployments should use pg_dump / the PITR tooling instead.
func (s *SQLStore) BackupTo(path string) error {
	if s.dialect != DialectSQLite {
		return ErrBackupUnsupported
	}
	_, err := s.exec(`VACUUM INTO ?`, path)
	return err
}
