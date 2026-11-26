package attach

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// extractXlsx reads every sheet of an xlsx workbook and concatenates the
// cells into a lightweight Markdown table per sheet.
func extractXlsx(data []byte) (string, error) {
	if err := checkZipExpansion(data); err != nil {
		return "", err
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer f.Close()

	var b strings.Builder
	sheets := f.GetSheetList()
	for si, sheet := range sheets {
		if si > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## %s\n\n", sheet)
		rows, err := f.GetRows(sheet)
		if err != nil {
			return "", err
		}
		for ri, row := range rows {
			if len(row) == 0 {
				continue
			}
			b.WriteString("| ")
			b.WriteString(strings.Join(row, " | "))
			b.WriteString(" |\n")
			if ri == 0 {
				// Markdown separator after the header row.
				b.WriteString(strings.Repeat("|---", len(row)))
				b.WriteString("|\n")
			}
		}
	}
	out := strings.TrimSpace(b.String())
	if extra, xerr := extractOfficeChartsAndDiagrams(data, "xl/charts/", "xl/diagrams/"); xerr == nil {
		out += extra
	}
	return out, nil
}
