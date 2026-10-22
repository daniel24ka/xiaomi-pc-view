package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Every T("...") text of the app has an English translation.
func TestTranslations(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`\bT\(("(?:[^"\\]|\\.)*")\)`)
	for _, f := range files {
		if strings.HasPrefix(f, "devtest") || strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllSubmatch(b, -1) {
			s, err := strconv.Unquote(string(m[1]))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := english[s]; !ok {
				t.Errorf("%s: no English for %q", f, s)
			}
		}
	}
}

// Every English text has a Hebrew one, with the same format verbs.
func TestHebrew(t *testing.T) {
	verbs := regexp.MustCompile(`%[-+# 0]*\d*(?:\.\d+)?[a-zA-Z]`)
	for pl, en := range english {
		he, ok := hebrew[en]
		if !ok {
			t.Errorf("no Hebrew for %q (%q)", en, pl)
			continue
		}
		if a, b := verbs.FindAllString(en, -1), verbs.FindAllString(he, -1); strings.Join(a, " ") != strings.Join(b, " ") {
			t.Errorf("verbs differ: %q %v / %q %v", en, a, he, b)
		}
	}
}
