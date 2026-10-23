// Command mobilebind is the MOBILE-RT desktop ChatOnce demo and bind entry documentation.
// For tools + HITL use the library APIs Send/Approve/Deny (see mobile/rt-shell).
//
// Desktop demo (no Android SDK required):
//
//	go run ./cmd/mobilebind -base http://127.0.0.1:18080/v1 -key demo -model mock -message "你好"
//
// gomobile (does not use cmd/baize):
//
//	# Android (JDK + NDK):  .\scripts\build-mobileagent-aar.ps1
//	# iOS (macOS + Xcode): ./scripts/build-mobileagent-ios.sh
//
// This binary only imports mobileagent (thin public bind) — proving the Run loop is local.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rebornace/baize/mobileagent"
)

func main() {
	base := flag.String("base", "", "OpenAI-compatible API base (e.g. https://api.openai.com/v1)")
	key := flag.String("key", "", "API key (Bearer); empty allowed for local mock")
	model := flag.String("model", "gpt-4o-mini", "model id")
	msg := flag.String("message", "", "user message; if empty, read one line from stdin")
	flag.Parse()

	if strings.TrimSpace(*base) == "" {
		fmt.Fprintln(os.Stderr, "mobilebind: -base is required (device/process talks to LLM directly; no baize serve)")
		flag.Usage()
		os.Exit(2)
	}

	text := strings.TrimSpace(*msg)
	if text == "" {
		var line string
		if _, err := fmt.Fscanln(os.Stdin, &line); err != nil {
			fmt.Fprintln(os.Stderr, "mobilebind: provide -message or stdin text")
			os.Exit(2)
		}
		text = line
	}

	if err := mobileagent.Configure(*base, *key, *model); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := mobileagent.ChatOnce(text)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(out)
}
