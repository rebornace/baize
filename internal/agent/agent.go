package agent

type Def struct {
	ID           string
	System       string
	Skills       []string
	ToolBinding  string   // store.ToolBindingFloor | store.ToolBindingExclusive; empty = floor
	ConnectorIDs []string // starter sample may use a single connector
}
