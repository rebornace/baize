package channel

// Outbound extra keys. These flow through the Deliver* -> Channel.Send*
// `extras` map. Channels that understand them (e.g. the out-of-process webhook
// channel) use them to tag outbound messages; channels that do not (weixin)
// simply ignore the extra keys, so setting them is always safe.
const (
	// ExtraKind classifies an outbound message. Values: OutboundKind*
	// below. Lets an external adapter render assistant replies, operator
	// (customer-service) mirror turns, and HITL/system notifications
	// distinctly. Default when unset is "assistant".
	ExtraKind = "kind"
	// ExtraRunID carries the baize run id an outbound message belongs to,
	// so an adapter can correlate replies/notifications with a run.
	ExtraRunID = "run_id"
)

// Outbound message kinds (ExtraKind values).
const (
	OutboundKindAssistant = "assistant" // engine assistant reply
	OutboundKindOperator  = "operator"  // UI/API operator turn mirrored to the peer
	OutboundKindNotify    = "notify"    // HITL approval / system notification
)

// WithOutboundMeta returns a copy of extras with the outbound kind and run id
// set. A nil/empty extras is allocated. Empty kind/runID are skipped so the
// channel-side defaults apply. The input map is never mutated.
func WithOutboundMeta(extras map[string]string, kind, runID string) map[string]string {
	out := make(map[string]string, len(extras)+2)
	for k, v := range extras {
		out[k] = v
	}
	if kind != "" {
		out[ExtraKind] = kind
	}
	if runID != "" {
		out[ExtraRunID] = runID
	}
	return out
}
