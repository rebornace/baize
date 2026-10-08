package decide

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	systemOnePath           = "/v1/systemone"
	systemOneDefaultTimeout = 8 * time.Second
	// DefaultSystemOneModel is the product default for Ollama's System One
	// endpoint (CPU-friendly 4B tev1). Override via DecideSystemOneModel for
	// nimble, tev1:0.8b, clef, Laya, or other compatible backends.
	DefaultSystemOneModel = "tev1"
	// noulAcceptFloor is the internal cutoff for treating a noul probability
	// as yes when mapping pick-many options. It is not a product knob and
	// never surfaces in Answer.
	noulAcceptFloor = 0.5
)

// SystemOne calls a TypeSafe-compatible decision endpoint (Ollama tev1 /
// nimble, Laya, Kev, Clef gateway, self-hosted, etc.) via POST /v1/systemone.
// Probabilities returned by the server are used only to pick a verdict /
// option and are discarded before Answer is returned (BAIZE-JEV-SPEC ADR).
//
// Empty BaseURL disables the implementation (Enabled=false) so the Chain
// falls through to chat RemoteMulti / Rules. Empty Model sends
// DefaultSystemOneModel (tev1).
type SystemOne struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

// NewSystemOne builds a System One client. baseURL may include a trailing
// slash; it is trimmed. A nil client uses an 8s timeout default.
func NewSystemOne(baseURL, apiKey, model string, client *http.Client) *SystemOne {
	if client == nil {
		client = &http.Client{Timeout: systemOneDefaultTimeout}
	}
	return &SystemOne{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
		Model:   strings.TrimSpace(model),
		Client:  client,
	}
}

// Enabled reports whether a base URL is configured.
func (s *SystemOne) Enabled() bool {
	return s != nil && s.BaseURL != ""
}

type systemOneRequest struct {
	Model     string                         `json:"model,omitempty"`
	State     string                         `json:"state"`
	Questions map[string]systemOneQuestion   `json:"questions"`
}

type systemOneQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions,omitempty"`
	Criteria     any               `json:"criteria,omitempty"`
}

type systemOneResponse struct {
	Answers map[string]json.RawMessage `json:"answers"`
}

type noulAnswer struct {
	Noul float64 `json:"noul"`
}

type choiceAnswer struct {
	Choice string `json:"choice"`
}

// Ask maps a baize Question onto System One primitives and returns an enum
// Answer. Network / schema failures yield ErrUnavailable.
func (s *SystemOne) Ask(ctx context.Context, q Question) (Answer, error) {
	if !s.Enabled() {
		return Answer{}, ErrUnavailable
	}
	questions, err := buildSystemOneQuestions(q)
	if err != nil {
		return Answer{}, ErrUnavailable
	}
	model := s.Model
	if model == "" {
		model = DefaultSystemOneModel
	}
	body := systemOneRequest{
		Model:     model,
		State:     strings.TrimSpace(q.Context),
		Questions: questions,
	}
	if body.State == "" {
		body.State = q.Kind
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Answer{}, ErrUnavailable
	}
	url := s.BaseURL + systemOnePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return Answer{}, ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	if s.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.APIKey)
	}
	res, err := s.Client.Do(req)
	if err != nil {
		return Answer{}, ErrUnavailable
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Answer{}, ErrUnavailable
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Answer{}, ErrUnavailable
	}
	var parsed systemOneResponse
	if err := json.Unmarshal(payload, &parsed); err != nil || len(parsed.Answers) == 0 {
		return Answer{}, ErrUnavailable
	}
	return mapSystemOneAnswers(q, parsed.Answers)
}

func buildSystemOneQuestions(q Question) (map[string]systemOneQuestion, error) {
	out := make(map[string]systemOneQuestion)
	switch {
	case len(q.Options) == 0:
		out["verdict"] = systemOneQuestion{
			Type:         "noul",
			Instructions: instructionsForKind(q.Kind),
		}
	case q.Kind == KindRouteTier || isPickOne(q):
		criteria := make(map[string]string, len(q.Options))
		for _, o := range q.Options {
			if d := strings.TrimSpace(q.Descriptions[o]); d != "" {
				criteria[o] = d
			} else {
				criteria[o] = o
			}
		}
		out["pick"] = systemOneQuestion{
			Type:         "choice",
			Instructions: instructionsForKind(q.Kind),
			Criteria:     criteria,
		}
	default:
		// Pick-many: one noul per option in a single round trip.
		for _, o := range q.Options {
			id := "opt_" + o
			inst := "Should this turn use option " + o + "?"
			if d := strings.TrimSpace(q.Descriptions[o]); d != "" {
				inst = "Should this turn use " + o + " (" + d + ")?"
			}
			out[id] = systemOneQuestion{Type: "noul", Instructions: inst}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no questions")
	}
	return out, nil
}

func isPickOne(q Question) bool {
	return q.Kind == KindRouteTier
}

func instructionsForKind(kind string) string {
	switch kind {
	case KindMemoryExtract:
		return "Does this turn contain durable facts worth remembering for this account?"
	case KindPruneToolResult:
		return "Is this tool result still worth keeping verbatim in later turns of the same run?"
	case KindRouteTier:
		return "Pick the better model tier for this turn: light (cheap/fast) or power (stronger reasoning)."
	case KindSystemTargets:
		return "Which backend systems does this turn need?"
	default:
		return "Answer the decision for this turn."
	}
}

func mapSystemOneAnswers(q Question, answers map[string]json.RawMessage) (Answer, error) {
	if len(q.Options) == 0 {
		raw, ok := answers["verdict"]
		if !ok {
			return Answer{}, ErrUnavailable
		}
		var n noulAnswer
		if err := json.Unmarshal(raw, &n); err != nil {
			return Answer{}, ErrUnavailable
		}
		v := VerdictNo
		if n.Noul >= noulAcceptFloor {
			v = VerdictYes
		}
		return Answer{Verdict: v, Source: SourceSystemOne}, nil
	}
	if isPickOne(q) {
		raw, ok := answers["pick"]
		if !ok {
			return Answer{}, ErrUnavailable
		}
		var c choiceAnswer
		if err := json.Unmarshal(raw, &c); err != nil || strings.TrimSpace(c.Choice) == "" {
			return Answer{}, ErrUnavailable
		}
		choice := strings.TrimSpace(c.Choice)
		allowed := false
		for _, o := range q.Options {
			if o == choice {
				allowed = true
				break
			}
		}
		if !allowed {
			return Answer{}, ErrUnavailable
		}
		return Answer{Verdict: VerdictYes, Value: choice, Source: SourceSystemOne}, nil
	}
	// Pick-many via per-option noul.
	allowed := make(map[string]bool, len(q.Options))
	for _, o := range q.Options {
		allowed[o] = true
	}
	values := make([]string, 0, len(q.Options))
	for _, o := range q.Options {
		raw, ok := answers["opt_"+o]
		if !ok {
			continue
		}
		var n noulAnswer
		if err := json.Unmarshal(raw, &n); err != nil {
			continue
		}
		if n.Noul >= noulAcceptFloor && allowed[o] {
			values = append(values, o)
		}
	}
	return Answer{Verdict: VerdictYes, Values: values, Source: SourceSystemOne}, nil
}
