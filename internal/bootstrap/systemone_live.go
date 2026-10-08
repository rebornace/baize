package bootstrap

import (
	"context"
	"net/http"
	"time"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/runtimecfg"
)

// systemOneLive reads System One endpoint settings from a knobs getter on every
// Ask so PATCH /settings/runtime applies without rewiring the decide chain.
type systemOneLive struct {
	knobs  func() runtimecfg.Knobs
	client *http.Client
}

func newSystemOneLive(knobs func() runtimecfg.Knobs) *systemOneLive {
	return &systemOneLive{
		knobs:  knobs,
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

func (s *systemOneLive) Enabled() bool {
	if s == nil || s.knobs == nil {
		return false
	}
	return decide.NewSystemOne(s.knobs().DecideSystemOneBaseURL, "", "", s.client).Enabled()
}

func (s *systemOneLive) Ask(ctx context.Context, q decide.Question) (decide.Answer, error) {
	if s == nil || s.knobs == nil {
		return decide.Answer{}, decide.ErrUnavailable
	}
	k := s.knobs()
	return decide.NewSystemOne(k.DecideSystemOneBaseURL, k.DecideSystemOneAPIKey, k.DecideSystemOneModel, s.client).Ask(ctx, q)
}
