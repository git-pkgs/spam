package textutil

import "unicode/utf8"

func Excerpt(data []byte) string {
	const maxBytes = 160
	end := min(len(data), maxBytes)
	for end < len(data) && !utf8.RuneStart(data[end]) {
		end--
	}
	return string(data[:end])
}
