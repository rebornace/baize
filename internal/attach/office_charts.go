package attach

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

const (
	maxOfficeExtraFiles     = 20
	maxOfficeExtraFileBytes = 1 << 20
	maxOfficeExtraChars     = 8000
)

// extractOfficeChartsAndDiagrams pulls readable chart data and SmartArt text
// from an Office package. Charts are not rendered as images; their cached
// categories/series values are appended as text instead.
func extractOfficeChartsAndDiagrams(data []byte, chartPrefix, diagramPrefix string) (string, error) {
	if err := checkZipExpansion(data); err != nil {
		return "", nil
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	charts, diagrams := 0, 0
	for _, f := range zr.File {
		if b.Len() >= maxOfficeExtraChars {
			break
		}
		name := strings.ToLower(f.Name)
		switch {
		case strings.HasPrefix(name, chartPrefix) && strings.HasSuffix(name, ".xml"):
			if charts >= maxOfficeExtraFiles {
				continue
			}
			txt := extractChartText(f)
			if txt != "" {
				b.WriteString("\n\n【图表数据（未渲染图形）】\n")
				b.WriteString(limitOfficeText(txt, 2000))
				charts++
			}
		case strings.HasPrefix(name, diagramPrefix) && strings.HasSuffix(name, ".xml"):
			if diagrams >= maxOfficeExtraFiles {
				continue
			}
			txt := extractSmartArtText(f)
			if txt != "" {
				b.WriteString("\n\n【SmartArt 文本】\n")
				b.WriteString(limitOfficeText(txt, 2000))
				diagrams++
			}
		}
	}
	return b.String(), nil
}

func readOfficeXML(f *zip.File) []byte {
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, maxOfficeExtraFileBytes+1))
	if err != nil || len(raw) > maxOfficeExtraFileBytes {
		return nil
	}
	return raw
}

type officeChartSeries struct {
	name string
	cats []string
	vals []string
}

func extractChartText(f *zip.File) string {
	raw := readOfficeXML(f)
	if len(raw) == 0 {
		return ""
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var out []officeChartSeries
	var cur *officeChartSeries
	section := ""
	inCache := false
	capturing := false
	var value strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF || err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "ser":
				out = append(out, officeChartSeries{})
				cur = &out[len(out)-1]
				section, inCache = "", false
			case "tx", "cat", "val":
				if cur != nil {
					section, inCache = t.Name.Local, false
				}
			case "strCache", "numCache":
				if cur != nil {
					inCache = true
				}
			case "v":
				if cur != nil && inCache && section != "" {
					capturing = true
					value.Reset()
				}
			}
		case xml.CharData:
			if capturing {
				value.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				if capturing && cur != nil {
					v := strings.TrimSpace(value.String())
					if v != "" {
						switch section {
						case "tx":
							if cur.name == "" {
								cur.name = v
							}
						case "cat":
							cur.cats = append(cur.cats, v)
						case "val":
							cur.vals = append(cur.vals, v)
						}
					}
					capturing = false
				}
			case "strCache", "numCache":
				inCache = false
			case "tx", "cat", "val":
				section = ""
			case "ser":
				cur = nil
			}
		}
	}
	var b strings.Builder
	for i, s := range out {
		if len(s.vals) == 0 {
			continue
		}
		name := s.name
		if name == "" {
			name = fmt.Sprintf("系列 %d", i+1)
		}
		b.WriteString(name)
		b.WriteString(": ")
		for j, v := range s.vals {
			if j > 0 {
				b.WriteString(", ")
			}
			if j < len(s.cats) && s.cats[j] != "" {
				b.WriteString(s.cats[j])
				b.WriteString("=")
			}
			b.WriteString(v)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func extractSmartArtText(f *zip.File) string {
	raw := readOfficeXML(f)
	if len(raw) == 0 {
		return ""
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var b strings.Builder
	seen := map[string]bool{}
	capturing := false
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF || err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				capturing = true
				text.Reset()
			}
		case xml.CharData:
			if capturing {
				text.Write(t)
			}
		case xml.EndElement:
			if t.Name.Local == "t" && capturing {
				v := strings.TrimSpace(text.String())
				if v != "" && !seen[v] {
					seen[v] = true
					if b.Len() > 0 {
						b.WriteString("\n")
					}
					b.WriteString(v)
				}
				capturing = false
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func limitOfficeText(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}
