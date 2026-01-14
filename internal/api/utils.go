package api

import (
	"path/filepath"
	"strings"
)

func getMimeType(filename string) string {
	ext := filepath.Ext(filename)
	lowerExt := strings.ToLower(ext)

	switch lowerExt {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".flv":
		return "video/x-flv"
	case ".webm":
		return "video/webm"
	case ".mpg", ".mpeg":
		return "video/mpeg"
	default:
		return "application/octet-stream"
	}
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func getDLNAProfile(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	commonProfile := "DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000"

	// Simplified profiles - some clients work better without PN
	switch ext {
	case ".mp4", ".m4v", ".mov":
		return commonProfile
	case ".mkv":
		return commonProfile
	case ".avi", ".divx":
		return commonProfile
	case ".wmv":
		return commonProfile
	case ".mpg", ".mpeg":
		return commonProfile
	default:
		return commonProfile
	}
}
