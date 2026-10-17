package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func marshalJSONStrings(v []string) (string, error) {
	if v == nil {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalJSONStrings(raw string, dst *[]string) error {
	if raw == "" || raw == "null" {
		*dst = nil
		return nil
	}
	return json.Unmarshal([]byte(raw), dst)
}

func marshalEvalChain(links []EvalChainLink) (string, error) {
	if links == nil {
		return "[]", nil
	}
	b, err := json.Marshal(links)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalEvalChain(raw string, dst *[]EvalChainLink) error {
	if raw == "" || raw == "null" {
		*dst = nil
		return nil
	}
	return json.Unmarshal([]byte(raw), dst)
}

func scanEvalCase(sc interface{ Scan(...any) error }) (EvalCase, error) {
	var c EvalCase
	var status, protocol, input, runStatus sql.NullString
	var requiredReads, consequential, expectAny, chain, eventsJSON, lastCodes, lastNotes sql.NullString
	var lastPass sql.NullInt64
	var lastScoredAt, createdAt, updatedAt sql.NullString
	if err := sc.Scan(
		&c.ID, &c.SourceRunID, &status, &protocol, &input,
		&requiredReads, &consequential, &expectAny, &chain, &eventsJSON, &runStatus,
		&lastPass, &lastCodes, &lastNotes, &lastScoredAt, &createdAt, &updatedAt,
	); err != nil {
		return EvalCase{}, err
	}
	c.Status = EvalCaseStatus(status.String)
	c.Protocol = protocol.String
	c.Input = input.String
	c.RunStatusAtIngest = runStatus.String
	c.LastScoreNotes = lastNotes.String
	if err := unmarshalJSONStrings(requiredReads.String, &c.RequiredReads); err != nil {
		return EvalCase{}, fmt.Errorf("required_reads: %w", err)
	}
	if err := unmarshalJSONStrings(consequential.String, &c.Consequential); err != nil {
		return EvalCase{}, fmt.Errorf("consequential: %w", err)
	}
	if err := unmarshalJSONStrings(expectAny.String, &c.ExpectConseqAny); err != nil {
		return EvalCase{}, fmt.Errorf("expect_conseq_any: %w", err)
	}
	if err := unmarshalEvalChain(chain.String, &c.Chain); err != nil {
		return EvalCase{}, fmt.Errorf("chain: %w", err)
	}
	if eventsJSON.Valid {
		c.EventsJSON = append([]byte(nil), eventsJSON.String...)
	}
	if lastCodes.Valid && lastCodes.String != "" {
		if err := unmarshalJSONStrings(lastCodes.String, &c.LastScoreCodes); err != nil {
			return EvalCase{}, fmt.Errorf("last_score_codes: %w", err)
		}
	}
	if lastPass.Valid {
		v := lastPass.Int64 != 0
		c.LastScorePass = &v
	}
	if lastScoredAt.Valid && lastScoredAt.String != "" {
		if t, err := parseStoreTime(lastScoredAt.String); err == nil {
			c.LastScoredAt = &t
		}
	}
	if createdAt.Valid {
		if t, err := parseStoreTime(createdAt.String); err == nil {
			c.CreatedAt = t
		}
	}
	if updatedAt.Valid {
		if t, err := parseStoreTime(updatedAt.String); err == nil {
			c.UpdatedAt = t
		}
	}
	return c, nil
}

func parseStoreTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
	}
	return t, err
}

const evalCaseSelectColumns = `id, source_run_id, status, protocol, input,
required_reads_json, consequential_json, expect_conseq_any_json, chain_json, events_json, run_status_at_ingest,
last_score_pass, last_score_codes_json, last_score_notes, last_scored_at, created_at, updated_at`

func (s *SQLStore) UpsertEvalCase(c EvalCase) error {
	requiredReads, err := marshalJSONStrings(c.RequiredReads)
	if err != nil {
		return err
	}
	consequential, err := marshalJSONStrings(c.Consequential)
	if err != nil {
		return err
	}
	expectAny, err := marshalJSONStrings(c.ExpectConseqAny)
	if err != nil {
		return err
	}
	chain, err := marshalEvalChain(c.Chain)
	if err != nil {
		return err
	}
	lastCodes, err := marshalJSONStrings(c.LastScoreCodes)
	if err != nil {
		return err
	}
	var lastPass sql.NullInt64
	if c.LastScorePass != nil {
		if *c.LastScorePass {
			lastPass = sql.NullInt64{Int64: 1, Valid: true}
		} else {
			lastPass = sql.NullInt64{Int64: 0, Valid: true}
		}
	}
	var lastScoredAt sql.NullString
	if c.LastScoredAt != nil {
		lastScoredAt = sql.NullString{String: c.LastScoredAt.UTC().Format(time.RFC3339Nano), Valid: true}
	}
	eventsCopy := append([]byte(nil), c.EventsJSON...)
	_, err = s.exec(
		`INSERT INTO eval_cases (`+evalCaseSelectColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   source_run_id=excluded.source_run_id, status=excluded.status, protocol=excluded.protocol, input=excluded.input,
		   required_reads_json=excluded.required_reads_json, consequential_json=excluded.consequential_json,
		   expect_conseq_any_json=excluded.expect_conseq_any_json, chain_json=excluded.chain_json,
		   events_json=excluded.events_json, run_status_at_ingest=excluded.run_status_at_ingest,
		   last_score_pass=excluded.last_score_pass, last_score_codes_json=excluded.last_score_codes_json,
		   last_score_notes=excluded.last_score_notes, last_scored_at=excluded.last_scored_at,
		   created_at=excluded.created_at, updated_at=excluded.updated_at`,
		c.ID, c.SourceRunID, string(c.Status), c.Protocol, c.Input,
		requiredReads, consequential, expectAny, chain, string(eventsCopy), c.RunStatusAtIngest,
		lastPass, lastCodes, c.LastScoreNotes, lastScoredAt,
		s.timeArg(c.CreatedAt), s.timeArg(c.UpdatedAt),
	)
	return err
}

func (s *SQLStore) GetEvalCase(id string) (EvalCase, error) {
	row := s.queryRow(`SELECT `+evalCaseSelectColumns+` FROM eval_cases WHERE id = ?`, id)
	c, err := scanEvalCase(row)
	if err == sql.ErrNoRows {
		return EvalCase{}, ErrEvalCaseNotFound
	}
	return c, err
}

func (s *SQLStore) ListEvalCases(limit int) ([]EvalCase, error) {
	limit = normalizeEvalListLimit(limit)
	rows, err := s.query(`SELECT `+evalCaseSelectColumns+` FROM eval_cases ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EvalCase
	for rows.Next() {
		c, err := scanEvalCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SQLStore) DeleteEvalCase(id string) error {
	res, err := s.exec(`DELETE FROM eval_cases WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrEvalCaseNotFound
	}
	return nil
}

func (s *SQLStore) FindEvalCaseBySourceRun(runID string) (EvalCase, bool, error) {
	row := s.queryRow(`SELECT `+evalCaseSelectColumns+` FROM eval_cases WHERE source_run_id = ? LIMIT 1`, runID)
	c, err := scanEvalCase(row)
	if err == sql.ErrNoRows {
		return EvalCase{}, false, nil
	}
	if err != nil {
		return EvalCase{}, false, err
	}
	return c, true, nil
}
