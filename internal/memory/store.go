package memory

// Store persists account-scoped memory entries.
type Store interface {
	Upsert(e Entry) (Entry, error)
	Get(id string) (Entry, error)
	Delete(ownerID, id string) error
	List(ownerID string, limit, offset int) ([]Entry, error)
	Search(ownerID, query string, topK int) ([]Entry, error)
	Forget(ownerID, key, text string) (int, error)
}
