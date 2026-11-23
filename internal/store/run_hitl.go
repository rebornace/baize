package store

// BeginHITLResume atomically claims a waiting_human run for one decision
// handler. The caller must release it (ReleaseHITLResume) if processing fails
// before the run reaches a terminal state.
func (s *Memory) BeginHITLResume(runID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.runs[runID]
	if r == nil || r.Status != StatusWaitingHuman {
		return false, nil
	}
	r.Status = StatusRunning
	r.Error = ""
	return true, nil
}

func (s *Memory) ReleaseHITLResume(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.runs[runID]
	if r != nil && r.Status == StatusRunning {
		r.Status = StatusWaitingHuman
	}
	return nil
}

func (s *Memory) ExpireRunIfWaiting(runID, reason string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.runs[runID]
	if r == nil || r.Status != StatusWaitingHuman {
		return false, nil
	}
	r.Status = StatusCancelled
	r.Output = ""
	r.Error = reason
	return true, nil
}

func (s *SQLStore) BeginHITLResume(runID string) (bool, error) {
	res, err := s.exec(`UPDATE runs SET status = ?, error = '' WHERE id = ? AND status = ?`,
		string(StatusRunning), runID, string(StatusWaitingHuman))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *SQLStore) ReleaseHITLResume(runID string) error {
	_, err := s.exec(`UPDATE runs SET status = ? WHERE id = ? AND status = ?`,
		string(StatusWaitingHuman), runID, string(StatusRunning))
	return err
}

func (s *SQLStore) ExpireRunIfWaiting(runID, reason string) (bool, error) {
	res, err := s.exec(`UPDATE runs SET status = ?, output = '', error = ? WHERE id = ? AND status = ?`,
		string(StatusCancelled), reason, runID, string(StatusWaitingHuman))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	return true, nil
}
