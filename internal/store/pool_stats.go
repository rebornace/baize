package store

// PoolStats is a snapshot of the SQL connection pool.
type PoolStats struct {
	MaxOpen   int
	Open      int
	InUse     int
	Idle      int
	WaitCount int64
}

// PoolStatter is implemented by stores backed by database/sql.
type PoolStatter interface {
	PoolStats() PoolStats
}

func (s *SQLStore) PoolStats() PoolStats {
	if s == nil || s.db == nil {
		return PoolStats{}
	}
	st := s.db.Stats()
	return PoolStats{
		MaxOpen:   st.MaxOpenConnections,
		Open:      st.OpenConnections,
		InUse:     st.InUse,
		Idle:      st.Idle,
		WaitCount: st.WaitCount,
	}
}

func (s *Memory) PoolStats() PoolStats { return PoolStats{} }
