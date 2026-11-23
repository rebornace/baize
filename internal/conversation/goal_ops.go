package conversation

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

func goalNow() time.Time {
	return time.Now().UTC()
}

func newGoalFrame(text string, criteria []string, details string, source GoalSource, now time.Time) (GoalFrame, error) {
	if err := validateGoalText(text, DefaultMaxGoalTextRunes); err != nil {
		return GoalFrame{}, err
	}
	if err := validateGoalCriteria(criteria); err != nil {
		return GoalFrame{}, err
	}
	if err := validateGoalDetails(details); err != nil {
		return GoalFrame{}, err
	}
	return GoalFrame{
		ID:        uuid.NewString(),
		Text:      text,
		Details:   details,
		Criteria:  cloneCriteria(criteria),
		Status:    GoalActive,
		Source:    source,
		Revision:  1,
		UpdatedAt: now,
	}, nil
}

// SetRoot resets the stack to a single active root frame.
func SetRoot(s GoalStack, conversationID, text string, criteria []string, source GoalSource) (GoalStack, error) {
	return SetRootDetailed(s, conversationID, text, "", criteria, source)
}

// SetRootDetailed is SetRoot with an optional details body (long context / constraints).
func SetRootDetailed(s GoalStack, conversationID, text, details string, criteria []string, source GoalSource) (GoalStack, error) {
	if conversationID == "" {
		return s, fmt.Errorf("conversation id required")
	}
	now := goalNow()
	frame, err := newGoalFrame(text, criteria, details, source, now)
	if err != nil {
		return s, err
	}
	s.ConversationID = conversationID
	s.Frames = []GoalFrame{frame}
	bumpGoalStack(&s, now)
	return s, nil
}

// Push appends a temporary active focus frame.
func Push(s GoalStack, text string, criteria []string, source GoalSource, maxDepth int) (GoalStack, error) {
	return PushDetailed(s, text, "", criteria, source, maxDepth)
}

// PushDetailed is Push with optional details.
func PushDetailed(s GoalStack, text, details string, criteria []string, source GoalSource, maxDepth int) (GoalStack, error) {
	if s.ConversationID == "" {
		return s, fmt.Errorf("goal stack is empty")
	}
	if maxDepth <= 0 {
		maxDepth = 8
	}
	if len(s.Frames) >= maxDepth {
		return s, fmt.Errorf("goal stack depth limit reached")
	}
	now := goalNow()
	frame, err := newGoalFrame(text, criteria, details, source, now)
	if err != nil {
		return s, err
	}
	s.Frames = append(s.Frames, frame)
	bumpGoalStack(&s, now)
	return s, nil
}

// Pop removes the stack top when it is interrupted (discards paused focus),
// otherwise removes the current active tip frame.
func Pop(s GoalStack) (GoalStack, error) {
	if len(s.Frames) == 0 {
		return s, fmt.Errorf("no active goal to pop")
	}
	now := goalNow()
	top := len(s.Frames) - 1
	if s.Frames[top].Status == GoalInterrupted {
		s.Frames = s.Frames[:top]
		bumpGoalStack(&s, now)
		return s, nil
	}
	idx, ok := activeTipIndex(s)
	if !ok {
		return s, fmt.Errorf("no active goal to pop")
	}
	s.Frames = append(s.Frames[:idx], s.Frames[idx+1:]...)
	bumpGoalStack(&s, now)
	return s, nil
}

// Stop marks the active tip as interrupted.
func Stop(s GoalStack) (GoalStack, error) {
	idx, ok := activeTipIndex(s)
	if !ok {
		return s, fmt.Errorf("no active goal to stop")
	}
	now := goalNow()
	s.Frames[idx].Status = GoalInterrupted
	bumpGoalFrame(&s.Frames[idx], now)
	bumpGoalStack(&s, now)
	return s, nil
}

// Resume reactivates the topmost interrupted frame.
func Resume(s GoalStack) (GoalStack, error) {
	idx, ok := interruptedTipIndex(s)
	if !ok {
		return s, fmt.Errorf("no interrupted goal to resume")
	}
	now := goalNow()
	s.Frames[idx].Status = GoalActive
	bumpGoalFrame(&s.Frames[idx], now)
	bumpGoalStack(&s, now)
	return s, nil
}

// PatchTip updates text and/or criteria on the active tip.
// Empty text keeps the current text; nil criteria keeps the current criteria.
func PatchTip(s GoalStack, text string, criteria []string) (GoalStack, error) {
	return PatchTipDetailed(s, text, criteria, nil)
}

// SetTipMaxTurns updates the active tip's turn budget. maxTurns=0 disables
// the budget and clears usage.
func SetTipMaxTurns(s GoalStack, maxTurns int) (GoalStack, error) {
	if maxTurns < 0 || maxTurns > 1000 {
		return s, fmt.Errorf("max_turns must be between 0 and 1000")
	}
	idx, ok := activeTipIndex(s)
	if !ok {
		return s, fmt.Errorf("no active goal")
	}
	s.Frames[idx].MaxTurns = maxTurns
	if maxTurns == 0 {
		s.Frames[idx].UsedTurns = 0
	}
	bumpGoalFrame(&s.Frames[idx], time.Now().UTC())
	bumpGoalStack(&s, time.Now().UTC())
	return s, nil
}

// IncrementTipTurn counts one LLM turn against the active tip.
func IncrementTipTurn(s GoalStack) (GoalStack, bool) {
	idx, ok := activeTipIndex(s)
	if !ok {
		return s, false
	}
	s.Frames[idx].UsedTurns++
	bumpGoalFrame(&s.Frames[idx], time.Now().UTC())
	bumpGoalStack(&s, time.Now().UTC())
	return s, true
}

// SetTipMaxTokens updates the active tip's token budget. maxTokens=0 disables
// the budget and clears usage. Used tokens are reported provider totals and
// may be estimates when the provider omits usage.
func SetTipMaxTokens(s GoalStack, maxTokens int) (GoalStack, error) {
	if maxTokens < 0 || maxTokens > MaxGoalTokenBudget {
		return s, fmt.Errorf("max_tokens must be between 0 and %d", MaxGoalTokenBudget)
	}
	idx, ok := activeTipIndex(s)
	if !ok {
		return s, fmt.Errorf("no active goal")
	}
	s.Frames[idx].MaxTokens = maxTokens
	if maxTokens == 0 {
		s.Frames[idx].UsedTokens = 0
	}
	now := time.Now().UTC()
	bumpGoalFrame(&s.Frames[idx], now)
	bumpGoalStack(&s, now)
	return s, nil
}

// AddTipTokens counts reported tokens against the active tip.
func AddTipTokens(s GoalStack, tokens int) (GoalStack, bool) {
	if tokens <= 0 {
		return s, false
	}
	idx, ok := activeTipIndex(s)
	if !ok {
		return s, false
	}
	s.Frames[idx].UsedTokens += tokens
	now := time.Now().UTC()
	bumpGoalFrame(&s.Frames[idx], now)
	bumpGoalStack(&s, now)
	return s, true
}

// PatchTipDetailed also updates details when details != nil (empty string clears).
func PatchTipDetailed(s GoalStack, text string, criteria []string, details *string) (GoalStack, error) {
	idx, ok := activeTipIndex(s)
	if !ok {
		return s, fmt.Errorf("no active goal tip")
	}
	newText := s.Frames[idx].Text
	if text != "" {
		if err := validateGoalText(text, DefaultMaxGoalTextRunes); err != nil {
			return s, err
		}
		newText = text
	}
	newCriteria := s.Frames[idx].Criteria
	if criteria != nil {
		if err := validateGoalCriteria(criteria); err != nil {
			return s, err
		}
		newCriteria = cloneCriteria(criteria)
	}
	newDetails := s.Frames[idx].Details
	if details != nil {
		if err := validateGoalDetails(*details); err != nil {
			return s, err
		}
		newDetails = *details
	}
	now := goalNow()
	s.Frames[idx].Text = newText
	s.Frames[idx].Criteria = newCriteria
	s.Frames[idx].Details = newDetails
	bumpGoalFrame(&s.Frames[idx], now)
	bumpGoalStack(&s, now)
	return s, nil
}
