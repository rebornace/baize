package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/hitllink"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

type hitlLinkProblem struct {
	status int
	zh     string
	en     string
}

func (s *Server) handleGetHITLLink(w http.ResponseWriter, r *http.Request) {
	locale := hitllink.LocaleFromRequest(r)
	claims, _, payload, problem := s.resolveHITLLink(r.PathValue("token"), time.Now())
	if problem != nil {
		s.writeHITLLinkMessage(w, problem.status, locale, hitlLinkText(locale, problem.zh, problem.en))
		return
	}
	s.writeHITLLinkHTML(w, http.StatusOK, hitllink.RenderPage(payload, claims, locale, ""))
}

func (s *Server) handlePostHITLLink(w http.ResponseWriter, r *http.Request) {
	locale := hitllink.LocaleFromRequest(r)
	_, runRec, _, problem := s.resolveHITLLink(r.PathValue("token"), time.Now())
	if problem != nil {
		s.writeHITLLinkMessage(w, problem.status, locale, hitlLinkText(locale, problem.zh, problem.en))
		return
	}

	if err := r.ParseForm(); err != nil {
		s.writeHITLLinkMessage(w, http.StatusBadRequest, locale, hitlLinkText(locale,
			"请求无效", "Invalid request"))
		return
	}
	decision := strings.TrimSpace(r.FormValue("decision"))
	if decision != "approve" && decision != "reject" {
		s.writeHITLLinkMessage(w, http.StatusBadRequest, locale, hitlLinkText(locale,
			"请选择同意或拒绝", "Choose approve or reject"))
		return
	}
	comment := strings.TrimSpace(r.FormValue("comment"))
	if len([]rune(comment)) > 2000 {
		s.writeHITLLinkMessage(w, http.StatusBadRequest, locale, hitlLinkText(locale,
			"备注过长", "Comment is too long"))
		return
	}

	job := middleware.Job{
		RunID:          runRec.ID,
		Kind:           middleware.KindHitlResume,
		AgentID:        runRec.AgentID,
		HITLApprove:    decision == "approve",
		HITLComment:    comment,
		HITLOperatorID: "hitl-link",
	}
	if s.Queue != nil {
		s.Dispatch(r.Context(), job)
	} else {
		_ = s.ExecuteJob(r.Context(), job)
	}

	updated, err := s.Store.GetRun(runRec.ID)
	if err != nil || updated == nil {
		s.writeHITLLinkMessage(w, http.StatusInternalServerError, locale, hitlLinkText(locale,
			"审批已提交，但读取状态失败", "Decision submitted, but status could not be read"))
		return
	}
	body := hitlLinkText(locale,
		"决定已提交，运行将继续。可回到 /ui 查看结果。",
		"Decision submitted. The run will continue; check /ui for the result.")
	if isTerminalStatus(updated.Status) {
		body = hitlLinkText(locale,
			fmt.Sprintf("该审批已处理，当前状态：%s。", updated.Status),
			fmt.Sprintf("This approval is handled. Current status: %s.", updated.Status))
	}
	s.writeHITLLinkMessage(w, http.StatusOK, locale, body)
}

func (s *Server) resolveHITLLink(token string, now time.Time) (hitllink.Claims, *store.Run, *store.HITLPayload, *hitlLinkProblem) {
	if len(s.CallbackSecret) == 0 {
		return hitllink.Claims{}, nil, nil, &hitlLinkProblem{
			status: http.StatusUnauthorized,
			zh:     "链接无效或已过期。请回到 /ui 或原聊天处理。",
			en:     "This link is invalid or expired. Use /ui or the original chat.",
		}
	}
	claims, err := hitllink.Verify(s.CallbackSecret, token, now)
	if err != nil {
		return hitllink.Claims{}, nil, nil, &hitlLinkProblem{
			status: http.StatusUnauthorized,
			zh:     "链接无效或已过期。请回到 /ui 或原聊天处理。",
			en:     "This link is invalid or expired. Use /ui or the original chat.",
		}
	}
	runRec, err := s.Store.GetRun(claims.RunID)
	if err != nil || runRec == nil {
		return hitllink.Claims{}, nil, nil, &hitlLinkProblem{
			status: http.StatusNotFound,
			zh:     "找不到这次审批对应的运行。",
			en:     "The run for this approval was not found.",
		}
	}
	if runRec.Status != store.StatusWaitingHuman {
		return hitllink.Claims{}, runRec, nil, &hitlLinkProblem{
			status: http.StatusConflict,
			zh:     "这次审批已处理或已过期。",
			en:     "This approval was already handled or has expired.",
		}
	}
	payload, err := s.Store.GetHITL(runRec.ID)
	if err != nil || payload == nil {
		return hitllink.Claims{}, runRec, nil, &hitlLinkProblem{
			status: http.StatusConflict,
			zh:     "这次审批已处理或已过期。",
			en:     "This approval was already handled or has expired.",
		}
	}
	if payload.Kind != "" && payload.Kind != store.HITLKindApproval {
		return hitllink.Claims{}, runRec, nil, &hitlLinkProblem{
			status: http.StatusConflict,
			zh:     "该链接只用于工具审批；登录等待请回到 /ui。",
			en:     "This link is only for tool approvals; use /ui for login waits.",
		}
	}
	wait := run.HITLWaitingSince(s.Store, runRec)
	if wait.IsZero() || wait.UTC().UnixNano() != claims.WaitUnixNano {
		return hitllink.Claims{}, runRec, nil, &hitlLinkProblem{
			status: http.StatusConflict,
			zh:     "链接已失效：这次等待已经被处理或被新的等待替换。",
			en:     "This link is stale: the wait was handled or replaced.",
		}
	}
	return claims, runRec, payload, nil
}

func hitlLinkText(locale, zh, en string) string {
	if strings.HasPrefix(strings.ToLower(locale), "en") {
		return en
	}
	return zh
}

func isTerminalStatus(status store.Status) bool {
	switch status {
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected:
		return true
	default:
		return false
	}
}

func (s *Server) writeHITLLinkMessage(w http.ResponseWriter, status int, locale, body string) {
	title := hitlLinkText(locale, "审批链接", "Approval link")
	s.writeHITLLinkHTML(w, status, hitllink.RenderMessagePage(locale, title, body))
}

func (s *Server) writeHITLLinkHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
