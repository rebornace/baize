package conversation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	DefaultMaxGoalTextRunes      = 500
	SoftMaxGoalTitleRunes        = 80
	DefaultMaxGoalDetailsRunes   = 2000
	DefaultMaxGoalCriteria       = 8
	DefaultMaxGoalCriterionRunes = 200
)

type GoalStatus string

const (
	GoalActive      GoalStatus = "active"
	GoalDone        GoalStatus = "done"
	GoalInterrupted GoalStatus = "interrupted"
	GoalSuperseded  GoalStatus = "superseded"
)

type GoalSource string

const (
	GoalSourceUser    GoalSource = "user"
	GoalSourceHarness GoalSource = "harness"
	GoalSourceAgent   GoalSource = "agent"
)

type GoalFrame struct {
	ID        string     `json:"id"`
	Text      string     `json:"text"`
	Details   string     `json:"details,omitempty"`
	Criteria  []string   `json:"criteria,omitempty"`
	Status    GoalStatus `json:"status"`
	Source    GoalSource `json:"source"`
	Revision  int        `json:"revision"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type GoalStack struct {
	ConversationID string      `json:"conversation_id"`
	Frames         []GoalFrame `json:"frames,omitempty"`
	Revision       int         `json:"revision"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// Root returns the bottom frame (主目标), if any.
func Root(s GoalStack) (GoalFrame, bool) {
	if len(s.Frames) == 0 {
		return GoalFrame{}, false
	}
	return s.Frames[0], true
}

// Tip returns the topmost active frame (当前焦点), if any.
func Tip(s GoalStack) (GoalFrame, bool) {
	for i := len(s.Frames) - 1; i >= 0; i-- {
		if s.Frames[i].Status == GoalActive {
			return s.Frames[i], true
		}
	}
	return GoalFrame{}, false
}

func activeTipIndex(s GoalStack) (int, bool) {
	for i := len(s.Frames) - 1; i >= 0; i-- {
		if s.Frames[i].Status == GoalActive {
			return i, true
		}
	}
	return -1, false
}

func interruptedTipIndex(s GoalStack) (int, bool) {
	for i := len(s.Frames) - 1; i >= 0; i-- {
		if s.Frames[i].Status == GoalInterrupted {
			return i, true
		}
	}
	return -1, false
}

// HasInterruptedFrame reports whether the stack contains any interrupted frame.
func HasInterruptedFrame(s GoalStack) bool {
	_, ok := interruptedTipIndex(s)
	return ok
}

// InterruptedTip returns the topmost interrupted frame, if any.
func InterruptedTip(s GoalStack) (GoalFrame, bool) {
	idx, ok := interruptedTipIndex(s)
	if !ok {
		return GoalFrame{}, false
	}
	return s.Frames[idx], true
}

func validateGoalText(text string, maxRunes int) error {
	if maxRunes <= 0 {
		maxRunes = DefaultMaxGoalTextRunes
	}
	if text == "" {
		return fmt.Errorf("goal text is required")
	}
	if utf8.RuneCountInString(text) > maxRunes {
		return fmt.Errorf("goal text is too long")
	}
	return nil
}

func validateGoalCriteria(criteria []string) error {
	if len(criteria) > DefaultMaxGoalCriteria {
		return fmt.Errorf("too many goal criteria")
	}
	for _, c := range criteria {
		if utf8.RuneCountInString(c) > DefaultMaxGoalCriterionRunes {
			return fmt.Errorf("goal criterion is too long")
		}
	}
	return nil
}

func validateGoalDetails(details string) error {
	if utf8.RuneCountInString(details) > DefaultMaxGoalDetailsRunes {
		return fmt.Errorf("goal details are too long")
	}
	return nil
}

// SplitGoalTitleAndDetails keeps a short goal title; overflow becomes details.
// Prefers a sentence boundary within SoftMaxGoalTitleRunes when present.
func SplitGoalTitleAndDetails(raw string) (title, details string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	runes := []rune(raw)
	if len(runes) <= SoftMaxGoalTitleRunes {
		return raw, ""
	}
	window := runes[:SoftMaxGoalTitleRunes]
	cut := -1
	// Prefer the first sentence boundary inside the soft title window.
	for i := 0; i < len(window); i++ {
		switch window[i] {
		case '。', '！', '？', '.', '!', '?', '\n', '；', ';':
			cut = i + 1
		}
		if cut >= 0 {
			break
		}
	}
	if cut <= 0 {
		cut = SoftMaxGoalTitleRunes
	}
	title = strings.TrimSpace(string(runes[:cut]))
	details = strings.TrimSpace(string(runes[cut:]))
	if title == "" {
		title = strings.TrimSpace(string(runes[:SoftMaxGoalTitleRunes]))
		details = strings.TrimSpace(string(runes[SoftMaxGoalTitleRunes:]))
	}
	return title, details
}

func cloneCriteria(criteria []string) []string {
	if len(criteria) == 0 {
		return nil
	}
	out := make([]string, len(criteria))
	copy(out, criteria)
	return out
}

func validateGoalStack(s GoalStack) error {
	if s.ConversationID == "" {
		return fmt.Errorf("conversation id required")
	}
	for _, f := range s.Frames {
		if f.ID == "" {
			return fmt.Errorf("goal frame id required")
		}
		if err := validateGoalText(f.Text, DefaultMaxGoalTextRunes); err != nil {
			return err
		}
		if err := validateGoalDetails(f.Details); err != nil {
			return err
		}
		if err := validateGoalCriteria(f.Criteria); err != nil {
			return err
		}
	}
	return nil
}

func bumpGoalStack(s *GoalStack, now time.Time) {
	s.Revision++
	s.UpdatedAt = now
}

func bumpGoalFrame(f *GoalFrame, now time.Time) {
	f.Revision++
	f.UpdatedAt = now
}

// EncodeGoalStack serializes a goal stack to JSON.
func EncodeGoalStack(s GoalStack) ([]byte, error) {
	return json.Marshal(s)
}

// DecodeGoalStack parses JSON into a GoalStack.
func DecodeGoalStack(raw []byte) (GoalStack, error) {
	var s GoalStack
	if err := json.Unmarshal(raw, &s); err != nil {
		return GoalStack{}, err
	}
	if err := validateGoalStack(s); err != nil {
		return GoalStack{}, err
	}
	return s, nil
}

// cloneGoalStackForFork deep-copies a goal stack for a forked conversation (new frame ids).
func cloneGoalStackForFork(src GoalStack, newConversationID string) GoalStack {
	if newConversationID == "" {
		return GoalStack{}
	}
	out := GoalStack{
		ConversationID: newConversationID,
		Revision:       src.Revision,
		UpdatedAt:      src.UpdatedAt,
	}
	if len(src.Frames) == 0 {
		return out
	}
	out.Frames = make([]GoalFrame, len(src.Frames))
	for i, f := range src.Frames {
		cp := f
		cp.ID = uuid.NewString()
		cp.Criteria = cloneCriteria(f.Criteria)
		out.Frames[i] = cp
	}
	return out
}
