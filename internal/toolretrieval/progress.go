package toolretrieval

// InstallProgress is reported while downloading / launching the local installer.
type InstallProgress struct {
	Event  string // "download" | "switch_mirror" | "launch" | "open_page"
	Mirror string // cn | official | github
	Bytes  int64
	Total  int64  // 0 if Content-Length unknown
	Note   string // human-readable reason when switching mirrors
}

// DownloadProgress is exposed on Status for the UI progress bar.
type DownloadProgress struct {
	Mirror  string `json:"mirror,omitempty"`
	Bytes   int64  `json:"bytes"`
	Total   int64  `json:"total"`
	Percent int    `json:"percent"` // 0–100; 0 when total unknown
	Note    string `json:"note,omitempty"`
}

func downloadPercent(bytes, total int64) int {
	if total <= 0 || bytes <= 0 {
		return 0
	}
	p := int(bytes * 100 / total)
	if p > 100 {
		return 100
	}
	if p < 0 {
		return 0
	}
	return p
}
