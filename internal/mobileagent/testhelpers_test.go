package mobileagent

// newTestAgent keeps the pre-netguard test behavior: local httptest servers
// are reachable. Network-policy tests use New() directly.
func newTestAgent() *Agent {
	a := New()
	a.SetAllowPrivateNetworks(true)
	return a
}
