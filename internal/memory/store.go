package memory

// Store persists account-scoped memory entries.
type Store interface {
	Upsert(e Entry) (Entry, error)
	// GetByOwnerKey returns the single entry for a non-empty key.
	// ok is false when the key is blank or no row exists.
	GetByOwnerKey(ownerID, key string) (Entry, bool, error)
	Get(id string) (Entry, error)
	Delete(ownerID, id string) error
	List(ownerID string, limit, offset int) ([]Entry, error)
	Search(ownerID, query string, topK int) ([]Entry, error)
	Forget(ownerID, key, text string) (int, error)
}
