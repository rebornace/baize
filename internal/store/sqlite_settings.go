package store

import "database/sql"

func (s *SQLStore) GetSetting(key string) ([]byte, bool, error) {
	var value sql.NullString
	err := s.queryRow(`SELECT value_json FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !value.Valid || value.String == "" {
		return nil, false, nil
	}
	return []byte(value.String), true, nil
}

func (s *SQLStore) UpsertSetting(key string, jsonRaw []byte) error {
	_, err := s.exec(
		`INSERT INTO settings (key, value_json) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json`,
		key, string(jsonRaw),
	)
	return err
}

func (s *SQLStore) ListSettingsByPrefix(prefix string) (map[string][]byte, error) {
	rows, err := s.query(
		`SELECT key, value_json FROM settings WHERE substr(key, 1, length(?)) = ?`,
		prefix, prefix,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var (
			key   string
			value sql.NullString
		)
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if !value.Valid || value.String == "" {
			continue
		}
		out[key] = []byte(value.String)
	}
	return out, rows.Err()
}
