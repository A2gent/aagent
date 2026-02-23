package llm

import "strings"

// DataURL builds a data URL from base64 image content when possible.
func (i Image) DataURL() string {
	mediaType := strings.TrimSpace(i.MediaType)
	data := strings.TrimSpace(i.DataBase64)
	if mediaType == "" || data == "" {
		return ""
	}
	return "data:" + mediaType + ";base64," + data
}
