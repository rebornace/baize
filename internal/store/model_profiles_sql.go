package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const upsertModelProfileColumns = `name, provider, base_url, model, api_key, api_key_env, disable_thinking, supports_vision, context_tokens, is_default, created_at, updated_at`

func scanModelProfile(scanner interface{ Scan(...any) error }) (ModelProfile, error) {
	var p ModelProfile
	var createdAt, updatedAt string
	var disableThinking, supportsVision, isDefault sql.NullBool
	if err := scanner.Scan(&p.ID, &p.Name, &p.Provider, &p.BaseURL, &p.Model, &p.APIKey,
		&p.APIKeyEnv, &disableThinking, &supportsVision, &p.ContextTokens, &isDefault, &createdAt, &updatedAt); err != nil {
		return ModelProfile{}, err
	}
	p.DisableThinking = disableThinking.Bool
	p.SupportsVision = supportsVision.Bool
	p.IsDefault = isDefault.Bool
	if t, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		p.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
		p.UpdatedAt = t
	}
	return p, nil
}

func (s *SQLStore) ListModelProfiles() ([]ModelProfile, error) {
	rows, err := s.query(`SELECT id, ` + upsertModelProfileColumns + ` FROM model_profiles ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelProfile
	for rows.Next() {
		p, err := scanModelProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLStore) GetModelProfile(id string) (ModelProfile, error) {
	row := s.queryRow(`SELECT id, `+upsertModelProfileColumns+` FROM model_profiles WHERE id = ?`, id)
	p, err := scanModelProfile(row)
	if err == sql.ErrNoRows {
		return ModelProfile{}, ErrModelProfileNotFound
	}
	return p, err
}

func (s *SQLStore) UpsertModelProfile(p ModelProfile) (ModelProfile, error) {
	if strings.TrimSpace(p.Name) == "" {
		return ModelProfile{}, fmt.Errorf("model profile name is required")
	}
	if strings.TrimSpace(p.Provider) == "" {
		p.Provider = "openai_compatible"
	}
	if p.ContextTokens <= 0 {
		p.ContextTokens = DefaultContextTokens // spec §6.1: never persist 0
	}
	now := time.Now().UTC()
	if p.ID == "" {
		p.ID = "mp_" + uuid.NewString()
		p.CreatedAt = now
		p.UpdatedAt = now
		_, err := s.exec(
			`INSERT INTO model_profiles (id, `+upsertModelProfileColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.Name, p.Provider, p.BaseURL, p.Model, p.APIKey, p.APIKeyEnv,
			p.DisableThinking, p.SupportsVision, p.ContextTokens, p.IsDefault,
			p.CreatedAt.Format(time.RFC3339Nano), p.UpdatedAt.Format(time.RFC3339Nano),
		)
		if err != nil {
			if isUniqueViolation(err) {
				return ModelProfile{}, fmt.Errorf("model profile name %q already exists", p.Name)
			}
			return ModelProfile{}, err
		}
		return p, nil
	}

	existing, err := s.GetModelProfile(p.ID)
	if err != nil {
		return ModelProfile{}, err
	}
	// Redacted/empty key must not overwrite the stored secret.
	if p.APIKey == "" || IsRedactedAPIKey(p.APIKey) {
		p.APIKey = existing.APIKey
	}
	p.IsDefault = existing.IsDefault // default only via SetDefaultModelProfile
	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = now
	_, err = s.exec(
		`UPDATE model_profiles SET name=?, provider=?, base_url=?, model=?, api_key=?, api_key_env=?,
		   disable_thinking=?, supports_vision=?, context_tokens=?, is_default=?, updated_at=? WHERE id=?`,
		p.Name, p.Provider, p.BaseURL, p.Model, p.APIKey, p.APIKeyEnv,
		p.DisableThinking, p.SupportsVision, p.ContextTokens, p.IsDefault,
		p.UpdatedAt.Format(time.RFC3339Nano), p.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ModelProfile{}, fmt.Errorf("model profile name %q already exists", p.Name)
		}
		return ModelProfile{}, err
	}
	return p, nil
}

func (s *SQLStore) DeleteModelProfile(id string) error {
	existing, err := s.GetModelProfile(id)
	if err != nil {
		return err
	}
	if existing.IsDefault {
		return fmt.Errorf("cannot delete the default model profile; set another as default first")
	}
	_, err = s.exec(`DELETE FROM model_profiles WHERE id = ?`, id)
	return err
}

func (s *SQLStore) SetDefaultModelProfile(id string) error {
	if _, err := s.GetModelProfile(id); err != nil {
		return err
	}
	if _, err := s.exec(`UPDATE model_profiles SET is_default = ?`, false); err != nil {
		return err
	}
	_, err := s.exec(`UPDATE model_profiles SET is_default = ? WHERE id = ?`, true, id)
	return err
}
