//go:build windows

package vision

func ocrInfo() map[string]any {
	return map[string]any{
		"available":      true,
		"engine":         "windows-worker",
		"activeLanguage": "system",
	}
}
