package bootstrap

import (
	"context"
	"strings"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

// routeProbeMaxRunes bounds the turn text sent to the DP-4 advisor; the
// arbitration probe must not cost more than the routing saving it enables.
const routeProbeMaxRunes = 1500

// defaultRouteMinRunes mirrors the baseline populated in runtime_settings.go;
// used only if the knob is missing.
const defaultRouteMinRunes = 400

// routeTierAdvisor adapts the decide layer to llm.TierAdvisor (DP-4). All
// trigger policy lives here and is read live from the hot knobs, so enabling
// the feature or changing the length floor takes effect without re-wiring.
type routeTierAdvisor struct {
	settings *runtimecfg.Holder
	decider  decide.Ask
}

// AdviseTier returns light/power only when the master + DP-4 switches are on,
// the turn meets the length floor, and the layer gives a usable answer. Every
// other case (off, short, degraded, unrecognized value) returns ok=false so the
// router keeps ClassifyTask's original standard tier.
func (a *routeTierAdvisor) AdviseTier(ctx context.Context, text string) (string, bool) {
	if a.settings == nil || a.decider == nil {
		return "", false
	}
	k := a.settings.Knobs()
	if !k.DecideEnabled || !k.DecideRouteEnabled {
		return "", false
	}
	minRunes := k.DecideRouteMinRunes
	if minRunes <= 0 {
		minRunes = defaultRouteMinRunes
	}
	text = strings.TrimSpace(text)
	if len([]rune(text)) < minRunes {
		return "", false
	}

	probe := text
	if r := []rune(probe); len(r) > routeProbeMaxRunes {
		probe = string(r[:routeProbeMaxRunes])
	}
	ans, err := a.decider.Ask(ctx, decide.Question{
		Kind:    decide.KindRouteTier,
		Context: "用户这一轮请求：\n" + probe,
		Options: []string{store.AutoTierLight, store.AutoTierPower},
		Descriptions: map[string]string{
			store.AutoTierLight: "便宜快速模型，适合闲聊/简单问答/格式转换等无需多步推理的任务",
			store.AutoTierPower: "更强更贵的推理模型，适合分析/规划/排查/长文本/多步/复杂代码",
		},
	})
	if err != nil || ans.Degraded {
		return "", false
	}
	tier := store.NormalizeAutoTier(ans.Value)
	if tier != store.AutoTierLight && tier != store.AutoTierPower {
		return "", false
	}
	return tier, true
}
