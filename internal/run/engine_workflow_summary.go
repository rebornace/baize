package run

import (
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/workflow"
)

// workflowCompletionSummary builds a user-facing summary and a compact event
// payload. Only allowlisted scalar fields are copied from tool results; raw
// responses never reach the chat or the event.
func workflowCompletionSummary(wf *workflow.Workflow, tree map[string]any) (string, []map[string]any) {
	return workflowSummary(wf, tree, "")
}

// workflowFailureSummary includes the failing step and marks unreached steps
// as pending instead of skipped.
func workflowFailureSummary(wf *workflow.Workflow, tree map[string]any, failedStep string) (string, []map[string]any) {
	return workflowSummary(wf, tree, failedStep)
}

func workflowSummary(wf *workflow.Workflow, tree map[string]any, failedStep string) (string, []map[string]any) {
	failedAt := -1
	for i, step := range wf.Steps {
		if step.ID == failedStep {
			failedAt = i
			break
		}
	}
	completed, skipped, failed, pending := 0, 0, 0, 0
	items := make([]map[string]any, 0, len(wf.Steps))
	lines := make([]string, 0, len(wf.Steps)+1)
	for i, step := range wf.Steps {
		item := map[string]any{"id": step.ID, "tool": step.Tool}
		node, _ := tree[step.ID].(map[string]any)
		result, hasResult := node["result"].(map[string]any)
		errKind, tolerated := node["error"].(string)

		switch {
		case step.ID == failedStep:
			failed++
			item["status"] = "failed"
			lines = append(lines, fmt.Sprintf("- %s (%s): failed", step.ID, step.Tool))
		case tolerated:
			failed++
			item["status"] = "failed"
			item["tolerated"] = true
			if errKind != "" {
				item["error"] = errKind
			}
			line := fmt.Sprintf("- %s (%s): failed (tolerated", step.ID, step.Tool)
			if errKind != "" {
				line += ", " + errKind
			}
			lines = append(lines, line+")")
		case hasResult:
			completed++
			item["status"] = "completed"
			for _, key := range []string{"id", "title", "name", "status"} {
				if v, ok := result[key]; ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						item["result_"+key] = truncateRunes(s, 80)
					}
				}
			}
			line := fmt.Sprintf("- %s (%s): completed", step.ID, step.Tool)
			if title, ok := item["result_title"].(string); ok && title != "" {
				line += " — " + title
			}
			lines = append(lines, line)
		case failedAt >= 0 && i > failedAt:
			pending++
			item["status"] = "pending"
			lines = append(lines, fmt.Sprintf("- %s (%s): not run", step.ID, step.Tool))
		default:
			skipped++
			item["status"] = "skipped"
			lines = append(lines, fmt.Sprintf("- %s (%s): skipped", step.ID, step.Tool))
		}
		items = append(items, item)
	}

	var head string
	if failedAt >= 0 || failedStep != "" {
		head = fmt.Sprintf("Workflow %q failed", wf.Name)
		if failedStep != "" {
			head += fmt.Sprintf(" at step %q", failedStep)
		}
		head += fmt.Sprintf(": %d completed, %d skipped", completed, skipped)
		if pending > 0 {
			head += fmt.Sprintf(", %d not run", pending)
		}
		if toleratedCount := failed - 1; toleratedCount > 0 {
			head += fmt.Sprintf(", %d tolerated failure(s)", toleratedCount)
		}
		head += "."
	} else if failed > 0 {
		head = fmt.Sprintf("Workflow %q finished with %d failed step(s): %d completed, %d skipped.", wf.Name, failed, completed, skipped)
	} else {
		head = fmt.Sprintf("Workflow %q completed: %d completed, %d skipped.", wf.Name, completed, skipped)
	}
	return strings.Join(append([]string{head}, lines...), "\n"), items
}

// workflowFailedStep extracts the step id from workflow engine errors, which
// wrap failures as `step "id": ...`.
func workflowFailedStep(err error) string {
	if err == nil {
		return ""
	}
	const prefix = `step "`
	msg := err.Error()
	if !strings.HasPrefix(msg, prefix) {
		return ""
	}
	rest := msg[len(prefix):]
	if i := strings.Index(rest, `"`); i > 0 {
		return rest[:i]
	}
	return ""
}
