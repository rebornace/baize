package artifact

type Store interface {
	PutHTML(runID string, html string) (id string, err error)
	Get(id string) (html string, runID string, err error)
}
