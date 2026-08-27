package agent

type Def struct {
	ID           string
	System       string
	Skills       []string
	ConnectorIDs []string // starter sample may use a single connector
}
