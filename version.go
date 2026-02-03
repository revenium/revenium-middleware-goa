package revenium

import (
	"fmt"
	"runtime/debug"
)

const middlewareName = "goa-ai-revenium"

var (
	middlewareVersion = "0.1.0"
	middlewareSource  string
	userAgent        string
)

func init() {
	goVersion := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		goVersion = info.GoVersion
	}
	middlewareSource = fmt.Sprintf("%s/%s", middlewareName, middlewareVersion)
	userAgent = fmt.Sprintf("%s/%s Go/%s", middlewareName, middlewareVersion, goVersion)
}
