// Package utils provides utility functions for template processing.
package utils

import "strings"

func Escape(in string) string {
	output := strings.Replace(in, "&", "&amp;", -1)
	output = strings.Replace(output, ">", "&gt;", -1)
	output = strings.Replace(output, "<", "&lt;", -1)
	output = strings.Replace(output, "\"", "&quot;", -1)
	output = strings.Replace(output, "'", "&#39;", -1)
	return output
}
