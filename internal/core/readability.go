package core

import (
	"strings"
	"unicode"
)

func syllables(w string) int {
	w = strings.ToLower(w)
	n, prev := 0, false
	for _, r := range w {
		v := strings.ContainsRune("aeiouy", r)
		if v && !prev {
			n++
		}
		prev = v
	}
	if strings.HasSuffix(w, "e") && !strings.HasSuffix(w, "le") && n > 1 {
		n--
	}
	if n == 0 {
		n = 1
	}
	return n
}

// GradeLevel is the Flesch-Kincaid grade level of a text.
func GradeLevel(text string) float64 {
	sentences := 0
	for _, r := range text {
		if r == '.' || r == '!' || r == '?' {
			sentences++
		}
	}
	if sentences == 0 {
		sentences = 1
	}
	words, syl := 0, 0
	for _, f := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' }) {
		words++
		hasLetter := false
		for _, r := range f {
			if unicode.IsLetter(r) {
				hasLetter = true
			}
		}
		if hasLetter {
			syl += syllables(f)
		} else {
			syl++
		}
	}
	if words == 0 {
		return 0
	}
	return 0.39*float64(words)/float64(sentences) + 11.8*float64(syl)/float64(words) - 15.59
}
