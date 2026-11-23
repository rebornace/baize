package store

func init() {
	RegisterDriver("sqlite", func(opts OpenOptions) (Store, error) {
		return OpenSQLite(opts.SQLitePath)
	})
}
