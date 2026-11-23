package tool

import "github.com/rebornace/baize/internal/llm"

// imagePartsKey is a reserved content-map key used to carry multimodal image
// results from a tool invoker back to the engine. It is stripped before the
// content is JSON-marshaled into the tool message text or persisted to events.
const imagePartsKey = "__baize_image_parts__"

// ImageResult pairs an image content part with its workspace-relative logical
// path. The engine uses Part to build the multimodal tool message and Path to
// record a lightweight image_refs pointer on the ToolResult event (bytes are
// never persisted to events).
type ImageResult struct {
	Path string
	Part llm.ContentPart
}

// WithImageParts returns a copy of content with image results attached under
// the reserved key. The original text keys are preserved. The engine reads
// them via ExtractImageParts. It does not mutate the input map.
func WithImageParts(content map[string]any, results ...ImageResult) map[string]any {
	out := make(map[string]any, len(content)+1)
	for k, v := range content {
		out[k] = v
	}
	if len(results) > 0 {
		out[imagePartsKey] = results
	}
	return out
}

// ExtractImageParts splits a tool content map into its cleaned text content
// (reserved key removed) and any attached image results. The returned cleaned
// map is safe to json.Marshal and to persist on events.
func ExtractImageParts(content map[string]any) (map[string]any, []ImageResult) {
	results := []ImageResult{}
	if raw, ok := content[imagePartsKey]; ok {
		if r, ok := raw.([]ImageResult); ok {
			results = r
		}
	}
	cleaned := make(map[string]any, len(content))
	for k, v := range content {
		if k == imagePartsKey {
			continue
		}
		cleaned[k] = v
	}
	return cleaned, results
}
