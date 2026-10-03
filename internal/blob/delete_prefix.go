package blob

import "context"

// DeletePrefix removes all objects whose keys start with prefix.
func DeletePrefix(ctx context.Context, s Store, prefix string) error {
	entries, err := s.List(ctx, prefix)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := s.Delete(ctx, e.Key); err != nil {
			return err
		}
	}
	return nil
}
