package store

import "fmt"

// Temporary stub implementations so *SQLStore satisfies the Store interface
// after task 1. Task 2 replaces these with the real SQL-backed CRUD.

func (s *SQLStore) UpsertModelProfile(p ModelProfile) (ModelProfile, error) {
	return ModelProfile{}, fmt.Errorf("not implemented")
}

func (s *SQLStore) GetModelProfile(id string) (ModelProfile, error) {
	return ModelProfile{}, fmt.Errorf("not implemented")
}

func (s *SQLStore) ListModelProfiles() ([]ModelProfile, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *SQLStore) DeleteModelProfile(id string) error {
	return fmt.Errorf("not implemented")
}

func (s *SQLStore) SetDefaultModelProfile(id string) error {
	return fmt.Errorf("not implemented")
}
