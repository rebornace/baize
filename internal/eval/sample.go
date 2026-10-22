package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/store"
)

const (
	SampleMaxNDefault = 10
	SampleMaxNHardCap = 20
	SampleModeDraft   = "draft"
	SampleModeScore   = "score"
)

// SampleInput is Admin one-shot online sampling (EV tail).
type SampleInput struct {
	Query  Query
	MaxN   int
	Seed   string
	Mode   string // draft | score
	Ingest IngestInput
}

// SampleError is one per-run failure during a batch.
type SampleError struct {
	RunID  string `json:"run_id"`
	Reason string `json:"reason"`
}

// SampleResult summarizes a sample batch.
type SampleResult struct {
	Mode       string        `json:"mode"`
	Candidates int           `json:"candidates"`
	Sampled    int           `json:"sampled"`
	Created    int           `json:"created"`
	Skipped    int           `json:"skipped"`
	Scored     int           `json:"scored"`
	Errors     []SampleError `json:"errors"`
	CaseIDs    []string      `json:"case_ids"`
}

// NormalizeSampleMaxN clamps max_n (default 10, hard cap 20).
func NormalizeSampleMaxN(n int) int {
	if n <= 0 {
		return SampleMaxNDefault
	}
	if n > SampleMaxNHardCap {
		return SampleMaxNHardCap
	}
	return n
}

// SelectSampleIDs picks up to maxN ids with deterministic order from seed+id hash.
func SelectSampleIDs(ids []string, maxN int, seed string) []string {
	maxN = NormalizeSampleMaxN(maxN)
	if len(ids) == 0 || maxN == 0 {
		return nil
	}
	type ranked struct {
		id  string
		key string
	}
	items := make([]ranked, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		sum := sha256.Sum256([]byte(seed + "\n" + id))
		items = append(items, ranked{id: id, key: hex.EncodeToString(sum[:])})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].key == items[j].key {
			return items[i].id < items[j].id
		}
		return items[i].key < items[j].key
	})
	if len(items) > maxN {
		items = items[:maxN]
	}
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return out
}

// SampleBatch queries candidates, samples, ingests, optionally scores.
func SampleBatch(st store.Store, in SampleInput) (SampleResult, error) {
	mode := strings.TrimSpace(in.Mode)
	if mode == "" {
		mode = SampleModeDraft
	}
	if mode != SampleModeDraft && mode != SampleModeScore {
		return SampleResult{}, errors.Join(ErrInvalidSample, errors.New("mode must be draft or score"))
	}

	ingestTemplate := in.Ingest
	ingestTemplate.Replace = false
	if mode == SampleModeScore {
		if !expectationsCompleteIngest(ingestTemplate) {
			return SampleResult{}, errors.Join(ErrInvalidSample, errors.New("score mode requires protocol and required_reads/consequential slices"))
		}
	}

	q := in.Query
	// Pull EV1a default window of candidates (up to 100) then sample.
	if q.Limit <= 0 {
		q.Limit = 100
	}
	cands, err := QueryCandidates(st, q)
	if err != nil {
		return SampleResult{}, err
	}
	ids := make([]string, 0, len(cands))
	for _, c := range cands {
		ids = append(ids, c.ID)
	}
	picked := SelectSampleIDs(ids, in.MaxN, in.Seed)

	out := SampleResult{
		Mode:       mode,
		Candidates: len(cands),
		Sampled:    len(picked),
		Errors:     []SampleError{},
		CaseIDs:    []string{},
	}

	for _, runID := range picked {
		_, found, err := st.FindEvalCaseBySourceRun(runID)
		if err != nil {
			out.Errors = append(out.Errors, SampleError{RunID: runID, Reason: err.Error()})
			continue
		}
		if found {
			out.Skipped++
			continue
		}

		ing := ingestTemplate
		ing.RunID = runID
		if mode == SampleModeDraft {
			// Leave expectations as provided; may stay draft if incomplete.
		}
		c, err := IngestFromRun(st, ing)
		if err != nil {
			if errors.Is(err, ErrConflict) {
				out.Skipped++
				continue
			}
			out.Errors = append(out.Errors, SampleError{RunID: runID, Reason: err.Error()})
			continue
		}
		out.Created++
		out.CaseIDs = append(out.CaseIDs, c.ID)

		if mode == SampleModeScore {
			if _, err := ScoreCase(st, c.ID); err != nil {
				out.Errors = append(out.Errors, SampleError{RunID: runID, Reason: "score: " + err.Error()})
				continue
			}
			out.Scored++
		}
	}
	return out, nil
}
