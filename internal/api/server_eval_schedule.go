package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/eval"
	"github.com/rebornace/baize/internal/webhook"
)

const evalGateAutoHourKey = "eval_gate_auto_hour"
const evalGateAutoLastDayKey = "eval_gate_auto_last_day"
const defaultEvalGateScheduleInterval = 10 * time.Minute

var evalGateScheduleMu sync.Mutex

func (s *Server) evalGateAutoHour() int {
	if s.Store == nil {
		return -1
	}
	raw, ok, err := s.Store.GetSetting(evalGateAutoHourKey)
	if err != nil || !ok {
		return -1
	}
	var hour int
	if json.Unmarshal(raw, &hour) != nil || hour < 0 || hour > 23 {
		return -1
	}
	return hour
}

// runNightlyGateIfDue runs the regression gate once per day at the configured
// UTC hour. It returns true when a run happened.
func (s *Server) runNightlyGateIfDue(now time.Time) (bool, error) {
	hour := s.evalGateAutoHour()
	if hour < 0 || now.UTC().Hour() != hour {
		return false, nil
	}
	day := now.UTC().Format("2006-01-02")
	if raw, ok, err := s.Store.GetSetting(evalGateAutoLastDayKey); err == nil && ok {
		var last string
		if json.Unmarshal(raw, &last) == nil && last == day {
			return false, nil
		}
	}
	res, err := eval.RunGate(s.Store, s.evalGateMinRate(), 0)
	if err != nil {
		return false, err
	}
	if raw, merr := json.Marshal(res); merr == nil {
		_ = s.Store.UpsertSetting(evalGateLastResultKey, raw)
	}
	dayRaw, _ := json.Marshal(day)
	_ = s.Store.UpsertSetting(evalGateAutoLastDayKey, dayRaw)
	if s.Webhook != nil {
		data := map[string]any{
			"pass": res.Pass, "empty": res.Empty, "total": res.Total,
			"passed": res.Passed, "failed": res.Failed,
			"pass_rate": res.PassRate, "min_pass_rate": res.MinPassRate,
		}
		if err := s.Webhook.SendAlert(context.Background(), "eval.gate", data); err != nil && !errors.Is(err, webhook.ErrNoURL) {
			log.Printf("eval gate schedule webhook: %v", err)
		}
	}
	return true, nil
}

// StartEvalGateSchedule checks every interval whether the nightly gate is due.
func (s *Server) StartEvalGateSchedule(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultEvalGateScheduleInterval
	}
	run := func() {
		if !evalGateScheduleMu.TryLock() {
			return
		}
		defer evalGateScheduleMu.Unlock()
		if _, err := s.runNightlyGateIfDue(time.Now().UTC()); err != nil {
			log.Printf("eval gate schedule: %v", err)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
