package specimport

import "testing"

func TestExtractEmbeddedSwaggerDoc(t *testing.T) {
	raw := `let options = { "swaggerDoc": {"openapi":"3.0.0","paths":{}}, "customOptions": {} };`
	doc, err := extractEmbeddedSwaggerDoc([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if DetectFormat(doc) != FormatOpenAPI3 {
		t.Fatalf("detect=%q want openapi3", DetectFormat(doc))
	}
}
