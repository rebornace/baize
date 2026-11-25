package attach_test

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/attach"
	"github.com/xuri/excelize/v2"
)

func mustReadFile(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("testdata", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return b
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// makeDocx builds a minimal docx zip carrying bodyXML in word/document.xml.
func makeDocx(bodyXML string) []byte {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	add := func(name, content string) {
		f, _ := w.Create(name)
		_, _ = f.Write([]byte(content))
	}
	add("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`)
	add("word/document.xml", bodyXML)
	_ = w.Close()
	return buf.Bytes()
}

func docxBody(text string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>%s</w:t></w:r></w:p></w:body></w:document>`, text)
}

// makeXlsx builds a small xlsx with one sheet.
func makeXlsx() []byte {
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "name")
	_ = f.SetCellValue("Sheet1", "B1", "age")
	_ = f.SetCellValue("Sheet1", "A2", "Alice")
	_ = f.SetCellValue("Sheet1", "B2", 30)
	buf, err := f.WriteToBuffer()
	if err != nil {
		panic(err)
	}
	_ = f.Close()
	return buf.Bytes()
}

// makePng builds a solid red w×h PNG.
func makePng(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func buildPDF(text string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	writeObj := func(n int, content string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, content)
	}
	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 5 0 R >> >> /MediaBox [0 0 612 792] /Contents 4 0 R >>")
	stream := ""
	if text != "" {
		stream = fmt.Sprintf("BT /F1 24 Tf 100 700 Td (%s) Tj ET", text)
	}
	writeObj(4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	writeObj(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	xrefStart := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(offsets))
	fmt.Fprintf(&b, "%010d 65535 f \r\n", 0)
	for _, off := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \r\n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(offsets), xrefStart)
	return b.Bytes()
}

// buildScannedPDF embeds one JPEG page image and no text operators, matching
// the common scanned-PDF shape.
func buildScannedPDF() []byte {
	var jpg bytes.Buffer
	_ = jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 2, 2)), &jpeg.Options{Quality: 80})
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	writeObj := func(n int, content string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, content)
	}
	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /Resources << /XObject << /Im0 5 0 R >> >> /MediaBox [0 0 612 792] /Contents 4 0 R >>")
	stream := "q 612 0 0 792 0 0 cm /Im0 Do Q"
	writeObj(4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	offsets = append(offsets, b.Len())
	fmt.Fprintf(&b, "5 0 obj\n<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", jpg.Len())
	b.Write(jpg.Bytes())
	b.WriteString("\nendstream\nendobj\n")
	xrefStart := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(offsets))
	fmt.Fprintf(&b, "%010d 65535 f \r\n", 0)
	for _, off := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \r\n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(offsets), xrefStart)
	return b.Bytes()
}

const (
	mimeTxt  = "text/plain"
	mimeMd   = "text/markdown"
	mimeCsv  = "text/csv"
	mimeDocx = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	mimeXlsx = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	mimePDF  = "application/pdf"
	mimePng  = "image/png"
	mimePptx = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	mimeHTML = "text/html"
	mimeZip  = "application/zip"
)

func TestProcessTextRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		mime     string
		path     string
	}{
		{"txt", "sample.txt", mimeTxt, "sample.txt"},
		{"md", "sample.md", mimeMd, "sample.md"},
		{"csv", "sample.csv", mimeCsv, "sample.csv"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := mustReadFile(t, c.path)
			texts, images, err := attach.Process([]attach.AttachmentIn{{
				Filename:   c.filename,
				MediaType:  c.mime,
				ContentB64: b64(data),
			}}, attach.DefaultOptions())
			if err != nil {
				t.Fatalf("Process: %v", err)
			}
			if len(images) != 0 {
				t.Fatalf("got %d images, want 0", len(images))
			}
			if len(texts) != 1 {
				t.Fatalf("got %d texts, want 1", len(texts))
			}
			got := texts[0].Text
			want := string(data)
			if got != want {
				t.Fatalf("text round-trip mismatch:\n got=%q\nwant=%q", got, want)
			}
			if texts[0].Kind != "text" {
				t.Fatalf("kind=%q want text", texts[0].Kind)
			}
			if texts[0].Filename != c.filename {
				t.Fatalf("filename=%q want %q", texts[0].Filename, c.filename)
			}
		})
	}
}

func TestProcessDocx(t *testing.T) {
	docx := makeDocx(docxBody("Hello docx body"))
	texts, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "a.docx",
		MediaType:  mimeDocx,
		ContentB64: b64(docx),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("got %d texts, want 1", len(texts))
	}
	if !strings.Contains(texts[0].Text, "Hello docx body") {
		t.Fatalf("docx text=%q, want substring", texts[0].Text)
	}
	if strings.Contains(texts[0].Text, "<") {
		t.Fatalf("docx text contains xml tag: %q", texts[0].Text)
	}
}

func TestProcessXlsx(t *testing.T) {
	xlsx := makeXlsx()
	texts, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "a.xlsx",
		MediaType:  mimeXlsx,
		ContentB64: b64(xlsx),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("got %d texts, want 1", len(texts))
	}
	if !strings.Contains(texts[0].Text, "Alice") || !strings.Contains(texts[0].Text, "30") {
		t.Fatalf("xlsx text=%q, want Alice and 30", texts[0].Text)
	}
}

func TestProcessPDFText(t *testing.T) {
	pdf := buildPDF("HelloPDF")
	texts, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "a.pdf",
		MediaType:  mimePDF,
		ContentB64: b64(pdf),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("got %d texts, want 1", len(texts))
	}
	if !strings.Contains(strings.TrimSpace(texts[0].Text), "HelloPDF") {
		t.Fatalf("pdf text=%q, want HelloPDF", texts[0].Text)
	}
}

func TestProcessPDFEmpty(t *testing.T) {
	pdf := buildPDF("")
	_, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "a.pdf",
		MediaType:  mimePDF,
		ContentB64: b64(pdf),
	}}, attach.DefaultOptions())
	if err == nil {
		t.Fatal("expected ErrEmptyPDFText, got nil")
	}
	if !errors.Is(err, attach.ErrEmptyPDFText) {
		t.Fatalf("err=%v, want ErrEmptyPDFText", err)
	}
}

func TestProcessPDFPassthrough(t *testing.T) {
	opts := attach.DefaultOptions()
	opts.AllowPDFPassthrough = true
	pdf := buildPDF("")
	texts, images, err := attach.Process([]attach.AttachmentIn{{
		Filename: "scan.pdf", MediaType: mimePDF, ContentB64: b64(pdf),
	}}, opts)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(texts) != 0 || len(images) != 1 {
		t.Fatalf("texts=%d images=%d", len(texts), len(images))
	}
	im := images[0]
	if im.Kind != "pdf" || im.ImageMIME != "application/pdf" || len(im.ImageBytes) != len(pdf) {
		t.Fatalf("pdf part=%+v", im)
	}
}

func TestProcessScannedPDFExtractsPageImages(t *testing.T) {
	texts, images, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "scan.pdf",
		MediaType:  mimePDF,
		ContentB64: b64(buildScannedPDF()),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(texts) != 1 || !strings.Contains(texts[0].Text, "扫描版 PDF") {
		t.Fatalf("texts=%+v", texts)
	}
	if len(images) != 1 {
		t.Fatalf("images=%d, want 1", len(images))
	}
	if !images[0].Document || images[0].ImageMIME != "image/jpeg" || len(images[0].ImageBytes) == 0 {
		t.Fatalf("image=%+v", images[0])
	}
	if images[0].Source != "scan.pdf" {
		t.Fatalf("source=%q, want scan.pdf", images[0].Source)
	}
}

func TestProcessDocxEmbeddedImages(t *testing.T) {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	add := func(name string, data []byte) {
		f, _ := w.Create(name)
		_, _ = f.Write(data)
	}
	add("word/document.xml", []byte(docxBody("")))
	add("word/media/image1.png", makePng(4, 4))
	_ = w.Close()
	texts, images, err := attach.Process([]attach.AttachmentIn{{
		Filename: "scan.docx", MediaType: mimeDocx, ContentB64: b64(buf.Bytes()),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 1 || !strings.Contains(texts[0].Text, "内嵌图片") {
		t.Fatalf("texts=%+v", texts)
	}
	if len(images) != 1 || !images[0].Document || images[0].ImageMIME != "image/png" {
		t.Fatalf("images=%+v", images)
	}
	if images[0].Source != "scan.docx" {
		t.Fatalf("source=%q, want scan.docx", images[0].Source)
	}
}

func TestProcessPngThumbnail(t *testing.T) {
	// 4000x3000 image, long edge > MaxImageEdge (2048) → must be shrunk.
	pngBig := makePng(3000, 4000)
	texts, images, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "a.png",
		MediaType:  mimePng,
		ContentB64: b64(pngBig),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(texts) != 0 || len(images) != 1 {
		t.Fatalf("got %d texts, %d images; want 0 texts, 1 image", len(texts), len(images))
	}
	img := images[0]
	if img.Kind != "image" {
		t.Fatalf("kind=%q want image", img.Kind)
	}
	if len(img.ImageBytes) == 0 {
		t.Fatal("thumbnail bytes empty")
	}
	if img.ImageMIME != mimePng {
		t.Fatalf("mime=%q want %q", img.ImageMIME, mimePng)
	}
	// Decode the thumbnail and check long edge <= 2048.
	cfg, err := png.DecodeConfig(bytes.NewReader(img.ImageBytes))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}
	long := cfg.Width
	if cfg.Height > long {
		long = cfg.Height
	}
	if long > 2048 {
		t.Fatalf("thumbnail long edge=%d, want <=2048", long)
	}
	if long < 1024 {
		t.Fatalf("thumbnail long edge=%d, want at least ~2048 (got too small)", long)
	}
}

func TestProcessUnsupported(t *testing.T) {
	_, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "a.xyz",
		MediaType:  "application/x-unknown",
		ContentB64: b64([]byte("data")),
	}}, attach.DefaultOptions())
	if err == nil {
		t.Fatal("expected ErrUnsupported, got nil")
	}
	if !errors.Is(err, attach.ErrUnsupported) {
		t.Fatalf("err=%v, want ErrUnsupported", err)
	}
}

func TestProcessTooMany(t *testing.T) {
	atts := make([]attach.AttachmentIn, 0, 6)
	for i := 0; i < 6; i++ {
		atts = append(atts, attach.AttachmentIn{
			Filename:   fmt.Sprintf("f%d.txt", i),
			MediaType:  mimeTxt,
			ContentB64: b64([]byte("x")),
		})
	}
	_, _, err := attach.Process(atts, attach.DefaultOptions())
	if err == nil {
		t.Fatal("expected ErrTooMany, got nil")
	}
	if !errors.Is(err, attach.ErrTooMany) {
		t.Fatalf("err=%v, want ErrTooMany", err)
	}
}

func TestProcessTooLarge(t *testing.T) {
	// Single attachment with decoded size > MaxTotalBytes (8 MiB).
	big := bytes.Repeat([]byte("A"), (8<<20)+1)
	_, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "big.txt",
		MediaType:  mimeTxt,
		ContentB64: b64(big),
	}}, attach.DefaultOptions())
	if err == nil {
		t.Fatal("expected ErrTooLarge, got nil")
	}
	if !errors.Is(err, attach.ErrTooLarge) {
		t.Fatalf("err=%v, want ErrTooLarge", err)
	}
}

func TestProcessTextTruncation(t *testing.T) {
	// MaxTextChars = 64 KiB. Provide 100 KiB of text.
	big := bytes.Repeat([]byte("B"), 100<<10)
	texts, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "big.txt",
		MediaType:  mimeTxt,
		ContentB64: b64(big),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("got %d texts, want 1", len(texts))
	}
	got := texts[0].Text
	// The content prefix is capped at 64 KiB runes; the truncation marker is
	// appended after that budget, so the total length is slightly over 64 KiB.
	const marker = "…[truncated]"
	if !strings.HasSuffix(got, marker) {
		t.Fatalf("truncated text missing %q marker; tail=%q", marker, got[len(got)-len(marker):])
	}
	prefix := strings.TrimSuffix(got, marker)
	if len([]rune(prefix)) != 64<<10 {
		t.Fatalf("content prefix rune count=%d, want %d", len([]rune(prefix)), 64<<10)
	}
	if len(prefix) < 1<<10 {
		t.Fatalf("prefix len=%d, want non-trivial", len(prefix))
	}
}

// TestAttachMaxTextCharsZeroNoTruncate proves MaxTextChars <= 0 disables
// truncation so callers can keep the full attachment body for workspace
// offload (AC-04).
func TestAttachMaxTextCharsZeroNoTruncate(t *testing.T) {
	big := bytes.Repeat([]byte("C"), 100<<10)
	texts, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "full.txt",
		MediaType:  mimeTxt,
		ContentB64: b64(big),
	}}, attach.Options{MaxTextChars: 0})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("got %d texts, want 1", len(texts))
	}
	got := texts[0].Text
	if len(got) != len(big) {
		t.Fatalf("Extracted.Text len=%d, want full %d", len(got), len(big))
	}
	if strings.Contains(got, "…[truncated]") {
		t.Fatalf("MaxTextChars=0 must not append truncation marker")
	}
}

// TestProcessTextTruncationRuneCount verifies MaxTextChars is interpreted as a
// rune (character) count, not a byte count, so multi-byte text is cut on a
// code-point boundary and never overruns the limit. The truncation marker is
// appended after the rune-budgeted prefix.
func TestProcessTextTruncationRuneCount(t *testing.T) {
	// 3 bytes per rune in UTF-8. 50k runes = 150k bytes > 64k rune limit.
	seg := "中文"                               // 2 runes, 6 bytes
	big := strings.Repeat(seg, 25_000)        // 50_000 runes
	opts := attach.Options{MaxTextChars: 100} // small limit for fast test
	texts, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "zh.txt",
		MediaType:  mimeTxt,
		ContentB64: b64([]byte(big)),
	}}, opts)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("got %d texts, want 1", len(texts))
	}
	const marker = "…[truncated]"
	got := texts[0].Text
	if !strings.HasSuffix(got, marker) {
		t.Fatalf("truncated text missing %q marker: %q", marker, got)
	}
	prefix := strings.TrimSuffix(got, marker)
	// The content prefix is exactly the rune budget; the marker is extra.
	if rn := utf8.RuneCountInString(prefix); rn != 100 {
		t.Fatalf("prefix rune count=%d, want 100", rn)
	}
	// Truncated text must be valid UTF-8 (no half-codepoint tail).
	if !utf8.ValidString(got) {
		t.Fatalf("truncated text is not valid UTF-8: %q", got)
	}
	// And the prefix must equal the corresponding prefix of the input.
	if prefix != string([]rune(big)[:100]) {
		t.Fatalf("truncated prefix does not match input prefix")
	}
}

// TestProcessTextNotTruncatedOmitsMarker verifies the marker is only added
// when truncation actually happens; an under-budget text is returned verbatim.
func TestProcessTextNotTruncatedOmitsMarker(t *testing.T) {
	small := strings.Repeat("a", 1000) // well under 64 KiB
	texts, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "small.txt",
		MediaType:  mimeTxt,
		ContentB64: b64([]byte(small)),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("got %d texts, want 1", len(texts))
	}
	if texts[0].Text != small {
		t.Fatalf("under-budget text was altered: got=%q want=%q", texts[0].Text, small)
	}
	if strings.Contains(texts[0].Text, "…[truncated]") {
		t.Fatalf("under-budget text must not carry the truncation marker: %q", texts[0].Text)
	}
}

// TestProcessZeroOptionsEqualsDefault proves a zero-valued Options is treated
// identically to DefaultOptions for every limit (count, bytes, text chars,
// image edge).
func TestProcessZeroOptionsEqualsDefault(t *testing.T) {
	pngBig := makePng(3000, 4000) // long edge > 2048 to exercise MaxImageEdge
	atts := []attach.AttachmentIn{
		{Filename: "a.txt", MediaType: mimeTxt, ContentB64: b64([]byte("hello"))},
		{Filename: "a.png", MediaType: mimePng, ContentB64: b64(pngBig)},
	}
	t1, i1, err1 := attach.Process(atts, attach.Options{})
	t2, i2, err2 := attach.Process(atts, attach.DefaultOptions())
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: zero=%v default=%v", err1, err2)
	}
	if len(t1) != len(t2) || len(i1) != len(i2) {
		t.Fatalf("counts differ: zero=(%d,%d) default=(%d,%d)", len(t1), len(i1), len(t2), len(i2))
	}
	if t1[0].Text != t2[0].Text {
		t.Fatalf("text differs: zero=%q default=%q", t1[0].Text, t2[0].Text)
	}
	if !bytes.Equal(i1[0].ImageBytes, i2[0].ImageBytes) {
		t.Fatalf("thumbnail bytes differ (zero vs default)")
	}
}

// TestProcessPartialOptionsKeepsCustom proves a partially populated Options
// keeps the caller's value for the set field while filling defaults for the
// rest. Here MaxCount=2 (custom) is honored → 3 attachments → ErrTooMany,
// while the other limits fall back to defaults so a normal-sized payload is
// not rejected as too large.
func TestProcessPartialOptionsKeepsCustom(t *testing.T) {
	atts := []attach.AttachmentIn{
		{Filename: "a.txt", MediaType: mimeTxt, ContentB64: b64([]byte("a"))},
		{Filename: "b.txt", MediaType: mimeTxt, ContentB64: b64([]byte("b"))},
		{Filename: "c.txt", MediaType: mimeTxt, ContentB64: b64([]byte("c"))},
	}
	_, _, err := attach.Process(atts, attach.Options{MaxCount: 2})
	if !errors.Is(err, attach.ErrTooMany) {
		t.Fatalf("err=%v, want ErrTooMany", err)
	}
}

// pngWithDeclaredSize builds a 1×1 PNG and rewrites its IHDR dimensions, so a
// tiny upload can claim an enormous decoded size without allocating it.
func pngWithDeclaredSize(t *testing.T, w, h uint32) []byte {
	t.Helper()
	b := makePng(1, 1)
	binary.BigEndian.PutUint32(b[16:20], w)
	binary.BigEndian.PutUint32(b[20:24], h)
	binary.BigEndian.PutUint32(b[29:33], crc32.ChecksumIEEE(b[12:29]))
	return b
}

func TestProcessImagePixelLimit(t *testing.T) {
	_, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "huge.png",
		MediaType:  mimePng,
		ContentB64: b64(pngWithDeclaredSize(t, 8000, 6000)),
	}}, attach.DefaultOptions())
	if !errors.Is(err, attach.ErrTooLarge) {
		t.Fatalf("err=%v, want ErrTooLarge", err)
	}
}

func TestProcessDocxExpansionLimit(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.CreateHeader(&zip.FileHeader{Name: "word/document.xml", Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	// 33 MiB of repetitive XML compresses to a few KB but expands past the cap.
	payload := strings.Repeat("a", 33<<20)
	if _, err := f.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = attach.Process([]attach.AttachmentIn{{
		Filename:   "bomb.docx",
		MediaType:  mimeDocx,
		ContentB64: b64(buf.Bytes()),
	}}, attach.DefaultOptions())
	if !errors.Is(err, attach.ErrTooLarge) {
		t.Fatalf("err=%v, want ErrTooLarge", err)
	}
}

func TestProcessPDFEmptyExplainsScannedFiles(t *testing.T) {
	_, _, err := attach.Process([]attach.AttachmentIn{{
		Filename:   "scan.pdf",
		MediaType:  mimePDF,
		ContentB64: b64(buildPDF("")),
	}}, attach.DefaultOptions())
	if !errors.Is(err, attach.ErrEmptyPDFText) {
		t.Fatalf("err=%v, want ErrEmptyPDFText", err)
	}
	if !strings.Contains(err.Error(), "scanned PDFs") {
		t.Fatalf("err=%q, want scanned-PDF guidance", err)
	}
}

func makePptx() []byte {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	add := func(name, content string) {
		f, _ := w.Create(name)
		_, _ = f.Write([]byte(content))
	}
	add("ppt/slides/slide1.xml", `<p:sld xmlns:p="p" xmlns:a="a"><a:t>Hello slide</a:t></p:sld>`)
	add("ppt/slides/slide2.xml", `<p:sld xmlns:p="p" xmlns:a="a"><a:t>Second slide</a:t></p:sld>`)
	add("ppt/notesSlides/notesSlide1.xml", `<p:notes xmlns:p="p" xmlns:a="a"><a:t>Speaker note</a:t></p:notes>`)
	_ = w.Close()
	return buf.Bytes()
}

func TestProcessPptx(t *testing.T) {
	texts, images, err := attach.Process([]attach.AttachmentIn{{
		Filename: "deck.pptx", MediaType: mimePptx, ContentB64: b64(makePptx()),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 1 || len(images) != 0 {
		t.Fatalf("texts=%d images=%d", len(texts), len(images))
	}
	for _, want := range []string{"Slide 1", "Hello slide", "Speaker note", "Slide 2", "Second slide"} {
		if !strings.Contains(texts[0].Text, want) {
			t.Fatalf("pptx text missing %q: %q", want, texts[0].Text)
		}
	}
}

func TestProcessHTML(t *testing.T) {
	page := []byte(`<html><head><title>T</title><script>bad()</script><style>.x{}</style></head><body><h1>Hi</h1><p>Hello <b>world</b></p></body></html>`)
	texts, _, err := attach.Process([]attach.AttachmentIn{{
		Filename: "page.html", MediaType: mimeHTML, ContentB64: b64(page),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 1 || !strings.Contains(texts[0].Text, "Hello world") {
		t.Fatalf("html=%+v", texts)
	}
	if strings.Contains(texts[0].Text, "bad()") || strings.Contains(texts[0].Text, ".x{}") {
		t.Fatalf("html leaked script/style: %q", texts[0].Text)
	}
}

func TestProcessZip(t *testing.T) {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	f, _ := w.Create("notes/a.txt")
	_, _ = f.Write([]byte("hello zip"))
	b, _ := w.Create("img.bin")
	_, _ = b.Write([]byte{0, 1, 2})
	evil, _ := w.Create("../evil.txt")
	_, _ = evil.Write([]byte("nope"))
	_ = w.Close()
	texts, _, err := attach.Process([]attach.AttachmentIn{{
		Filename: "bundle.zip", MediaType: mimeZip, ContentB64: b64(buf.Bytes()),
	}}, attach.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 1 {
		t.Fatalf("texts=%+v", texts)
	}
	text := texts[0].Text
	for _, want := range []string{"## notes/a.txt", "hello zip", "img.bin (binary, not extracted)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("zip text missing %q: %q", want, text)
		}
	}
	if strings.Contains(text, "evil.txt") {
		t.Fatalf("zip traversal entry leaked: %q", text)
	}
}

func TestSupportsNewDocumentTypes(t *testing.T) {
	for _, m := range []string{mimePptx, mimeHTML, mimeZip, "application/x-zip-compressed", "application/xhtml+xml"} {
		if !attach.SupportsMediaType(m) {
			t.Fatalf("SupportsMediaType(%q)=false", m)
		}
	}
}
