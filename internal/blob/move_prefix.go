package blob

import (
	"context"
	"fmt"
	"strings"
)

// MovePrefix copies every object under fromPrefix to toPrefix (replacing the
// from prefix on each key), then deletes the source objects. Destination keys
// overwrite if they already exist. fromPrefix/toPrefix should end with "/".
func MovePrefix(ctx context.Context, s Store, fromPrefix, toPrefix string) error {
	if s == nil {
		return fmt.Errorf("blob store required")
	}
	if fromPrefix == "" || toPrefix == "" {
		return fmt.Errorf("prefixes required")
	}
	entries, err := s.List(ctx, fromPrefix)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("no objects under %s", fromPrefix)
	}
	for _, e := range entries {
		rel := strings.TrimPrefix(e.Key, fromPrefix)
		data, err := s.Get(ctx, e.Key)
		if err != nil {
			return err
		}
		if err := s.Put(ctx, toPrefix+rel, data, ""); err != nil {
			return err
		}
	}
	return DeletePrefix(ctx, s, fromPrefix)
}
