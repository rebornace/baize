package bootstrap

import (
	"context"

	"github.com/rebornace/baize/internal/llm"
)

// errNoDecideProfile reports that no dedicated decision profile is configured.
var errNoDecideProfile = &decideProfileErr{"decide: no decide_profile_id configured"}

type decideProfileErr struct{ msg string }

func (e *decideProfileErr) Error() string { return e.msg }

// decideProfileProvider pins every Chat to the decision-layer model profile
// returned by profileID. It decouples decision calls from the run's main model
// even though both flow through the same llm.Switch: the Switch reads the
// profile id from the context this wrapper injects.
//
// When profileID returns "" (no dedicated decision profile configured) Chat
// returns an error. decide.RemoteMulti maps that to ErrUnavailable, so the
// Chain degrades to Rules/fallback rather than silently billing the main
// model. An operator configures the id at runtime via decide_profile_id.
type decideProfileProvider struct {
	next      llm.Provider
	profileID func() string
}

func (p decideProfileProvider) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	id := p.profileID()
	if id == "" {
		return llm.Message{}, errNoDecideProfile
	}
	ctx = llm.WithModelProfileID(ctx, id)
	return p.next.Chat(ctx, msgs, tools)
}

func (p decideProfileProvider) SupportsVision() bool { return false }
