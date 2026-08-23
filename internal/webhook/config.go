package webhook

// Config is the global events webhook configuration shape.
type Config struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}
