package attach

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var pptxSlideNum = regexp.MustCompile(`(?:^|/)slide(\d+)\.xml$`)
var pptxNotesNum = regexp.MustCompile(`(?:^|/)notesSlide(\d+)\.xml$`)

// extractPptx reads slide text and speaker notes from a PPTX package. Images
// and embedded objects are not extracted; only readable text is returned.
func extractPptx(data []byte) (string, error) {
	if err := checkZipExpansion(data); err != nil {
		return "", err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	type slide struct {
		num   int
		text  string
		notes string
	}
	byNum := map[int]*slide{}
	for _, f := range zr.File {
		if m := pptxSlideNum.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			t, rerr := readZipText(f)
			if rerr != nil {
				return "", rerr
			}
			if byNum[n] == nil {
				byNum[n] = &slide{num: n}
			}
			byNum[n].text = stripXMLTags(t)
			continue
		}
		if m := pptxNotesNum.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			t, rerr := readZipText(f)
			if rerr != nil {
				return "", rerr
			}
			if byNum[n] == nil {
				byNum[n] = &slide{num: n}
			}
			byNum[n].notes = stripXMLTags(t)
		}
	}
	if len(byNum) == 0 {
		return "", fmt.Errorf("attach: pptx has no slides")
	}
	nums := make([]int, 0, len(byNum))
	for n := range byNum {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	var b strings.Builder
	for _, n := range nums {
		s := byNum[n]
		if strings.TrimSpace(s.text) == "" && strings.TrimSpace(s.notes) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## Slide %d\n", n)
		b.WriteString(strings.TrimSpace(s.text))
		if strings.TrimSpace(s.notes) != "" {
			b.WriteString("\n\nNotes: ")
			b.WriteString(strings.TrimSpace(s.notes))
		}
	}
	if extra, xerr := extractOfficeChartsAndDiagrams(data, "ppt/charts/", "ppt/diagrams/"); xerr == nil {
		b.WriteString(extra)
	}
	return b.String(), nil
}

func readZipText(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, maxDocUncompressed+1))
	if err != nil {
		return "", err
	}
	if len(raw) > maxDocUncompressed {
		return "", fmt.Errorf("%w: document expands beyond %d bytes", ErrTooLarge, maxDocUncompressed)
	}
	return string(raw), nil
}
