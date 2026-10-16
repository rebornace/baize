package schedule

import (
	"context"
	"fmt"
	"time"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/tool"
)

// MetaReader resolves the owning account for a conversation (same semantics
// as conversation.MetaStore + memory.ownerFromCtx, without importing conversation).
type MetaReader interface {
	GetMeta(conversationID string) (ownerID string, err error)
}

const CreateName = "schedule_create"

// ToolMeta pairs the schedule_create spec with its invoker for registration.
type ToolMeta struct {
	Spec    llm.ToolSpec
	Invoker tool.Invoker
}

// CreateSpec is the LLM tool schema for creating a conversation schedule.
func CreateSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        CreateName,
		Description: "Create a one-shot, daily, or weekly reminder in the current conversation. Requires human approval before it is saved.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind": map[string]any{
					"type":        "string",
					"description": "once, daily, or weekly",
					"enum":        []any{"once", "daily", "weekly"},
				},
				"instruction": map[string]any{
					"type":        "string",
					"description": "Instruction to run when the schedule fires",
				},
				"timezone": map[string]any{
					"type":        "string",
					"description": "IANA timezone name; optional (defaults to server local)",
				},
				"fire_at": map[string]any{
					"type":        "string",
					"description": "Required for once: RFC3339 instant strictly after now",
				},
				"time_of_day": map[string]any{
					"type":        "string",
					"description": "Required for daily and weekly: HH:MM in the schedule timezone",
				},
				"weekday": map[string]any{
					"type":        "integer",
					"description": "Required for weekly: 1=Monday … 7=Sunday",
				},
			},
			"required": []string{"kind", "instruction"},
		},
	}
}

// CreateToolMeta builds schedule_create for RegisterSpecApproved(..., true).
// now supplies the clock for validation at invoke time; nil uses time.Now.
func CreateToolMeta(store Store, meta MetaReader, now func() time.Time) ToolMeta {
	if now == nil {
		now = time.Now
	}
	return ToolMeta{
		Spec:    CreateSpec(),
		Invoker: CreateInvoker(store, meta, now),
	}
}

// ValidateCreateArgs checks tool arguments before HITL; invalid args must not enter waiting_human.
func ValidateCreateArgs(args map[string]any, now time.Time) error {
	d, err := draftFromArgs(args)
	if err != nil {
		return err
	}
	_, err = Validate(d, now)
	return err
}

// CreateInvoker inserts an active schedule after HITL approval.
func CreateInvoker(store Store, meta MetaReader, now func() time.Time) tool.Invoker {
	if now == nil {
		now = time.Now
	}
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		owner, errPayload, ok := ownerFromCtx(ctx, meta)
		if !ok {
			return errPayload, true, nil
		}
		conv := identity.ConversationIDFrom(ctx)
		if conv == "" {
			return map[string]any{"error": "schedule requires a conversation context"}, true, nil
		}
		d, err := draftFromArgs(args)
		if err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		invokeNow := now()
		next, err := Validate(d, invokeNow)
		if err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		it, err := store.Insert(Item{
			OwnerID:        owner,
			ConversationID: conv,
			Instruction:    d.Instruction,
			Kind:           d.Kind,
			Timezone:       d.Timezone,
			FireAt:         d.FireAt,
			TimeOfDay:      d.TimeOfDay,
			Weekday:        d.Weekday,
			Status:         StatusActive,
			NextRunAt:      next.UTC(),
		})
		if err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		out := map[string]any{
			"ok":          true,
			"id":          it.ID,
			"next_run_at": it.NextRunAt.Format(time.RFC3339),
			"status":      it.Status,
		}
		return out, false, nil
	}
}

func draftFromArgs(args map[string]any) (Draft, error) {
	kind, ok := stringArg(args, "kind")
	if !ok {
		return Draft{}, fmt.Errorf("schedule: kind required")
	}
	instruction, ok := stringArg(args, "instruction")
	if !ok {
		return Draft{}, fmt.Errorf("schedule: instruction required")
	}
	tz, _ := stringArgOptional(args, "timezone")
	d := Draft{Kind: kind, Instruction: instruction, Timezone: tz}
	switch kind {
	case "once":
		fireRaw, ok := stringArg(args, "fire_at")
		if !ok {
			return Draft{}, fmt.Errorf("schedule: fire_at required for once")
		}
		fire, err := parseFireAt(fireRaw)
		if err != nil {
			return Draft{}, err
		}
		d.FireAt = fire
	case "daily":
		tod, ok := stringArg(args, "time_of_day")
		if !ok {
			return Draft{}, fmt.Errorf("schedule: time_of_day required for daily")
		}
		d.TimeOfDay = tod
	case "weekly":
		tod, ok := stringArg(args, "time_of_day")
		if !ok {
			return Draft{}, fmt.Errorf("schedule: time_of_day required for weekly")
		}
		wd, ok := intArg(args, "weekday")
		if !ok {
			return Draft{}, fmt.Errorf("schedule: weekday required for weekly")
		}
		d.TimeOfDay = tod
		d.Weekday = wd
	default:
		return Draft{}, fmt.Errorf("schedule: invalid kind %q", kind)
	}
	return d, nil
}

func parseFireAt(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("schedule: fire_at must be RFC3339")
	}
	return t, nil
}

func stringArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

func stringArgOptional(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func intArg(args map[string]any, key string) (int, bool) {
	v, ok := args[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func ownerFromCtx(ctx context.Context, meta MetaReader) (string, map[string]any, bool) {
	conv := identity.ConversationIDFrom(ctx)
	if conv == "" {
		return "", map[string]any{"error": "schedule requires a conversation context"}, false
	}
	if meta == nil {
		return "", map[string]any{"error": "schedule meta store unavailable"}, false
	}
	owner, err := meta.GetMeta(conv)
	if err != nil {
		return "", map[string]any{"error": fmt.Sprintf("conversation meta: %v", err)}, false
	}
	if owner == "" {
		return "local-dev", nil, true
	}
	return owner, nil, true
}
