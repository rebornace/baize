package attach

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
)

// extractOfficeImages pulls embedded raster images from an Office Open XML
// package (docx/pptx/xlsx). Charts and embedded objects are not images and are
// skipped. Limits are shared with scanned-PDF page images.
func extractOfficeImages(data []byte, prefix string, maxEdge int) []Extracted {
	if err := checkZipExpansion(data); err != nil {
		return nil
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}
	var files []*zip.File
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, prefix) || strings.HasSuffix(f.Name, "/") {
			continue
		}
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	var out []Extracted
	total := 0
	for _, f := range files {
		if len(out) >= maxDocumentImages || total >= maxDocumentImageBytes {
			break
		}
		if f.UncompressedSize64 == 0 || f.UncompressedSize64 > uint64(maxDocumentImageBytes-total) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, rerr := io.ReadAll(io.LimitReader(rc, int64(maxDocumentImageBytes-total)))
		_ = rc.Close()
		if rerr != nil || len(b) == 0 {
			continue
		}
		mime := http.DetectContentType(b)
		if !isImageMIME(mime) {
			continue
		}
		mime, thumb, perr := processImage(b, mime, maxEdge)
		if perr != nil || len(thumb) == 0 {
			continue
		}
		total += len(thumb)
		out = append(out, Extracted{
			Filename: path.Base(f.Name), Kind: "image",
			ImageMIME: mime, ImageBytes: thumb, Document: true,
		})
	}
	return out
}

func documentImagesPlaceholder(count int) string {
	return fmt.Sprintf("（文档未检测到文字层，已提取 %d 张内嵌图片，请从图片读取内容。）", count)
}
