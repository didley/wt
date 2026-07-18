//go:build !linux

package main

// disableWebviewZoom is a no-op outside Linux. macOS's WKWebView doesn't
// enable magnification by default, and Windows' WebView2 zoom is disabled
// via options.App.Windows in main.go instead.
func disableWebviewZoom() {}
