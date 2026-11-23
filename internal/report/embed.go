package report

import _ "embed"

//go:embed shell.html
var shellHTML string

//go:embed assets/echarts.min.js
var echartsJS string

//go:embed runtime.js
var runtimeJS string
