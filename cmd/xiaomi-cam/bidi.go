package main

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// Fyne lays text out left to right only (no bidi), so Hebrew is given to it in visual
// order: visual reverses each line, keeps Latin/number runs (and fmt verbs) in their own
// order and mirrors brackets. Format verbs get explicit indexes first (%d → %[1]d), so
// Sprintf still takes its arguments in logical order after the verbs moved.

var fmtVerb = regexp.MustCompile(`%(?:\[\d+\])?[-+# 0]*\d*(?:\.\d+)?[a-zA-Z]|%%`)

// rtlWrap - longer texts are broken into lines here: Fyne would wrap the visual string
// from its left end, i.e. starting with the end of the sentence.
const rtlWrap = 48

func isRTL(r rune) bool {
	return r >= 0x0590 && r <= 0x08FF || r >= 0xFB1D && r <= 0xFDFF || r >= 0xFE70 && r <= 0xFEFF
}

func isLTR(r rune) bool {
	return !isRTL(r) && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

var mirror = map[rune]rune{'(': ')', ')': '(', '[': ']', ']': '[', '{': '}', '}': '{',
	'<': '>', '>': '<', '«': '»', '»': '«'}

func indexVerbs(s string) string {
	n := 0
	return fmtVerb.ReplaceAllStringFunc(s, func(v string) string {
		if v == "%%" || strings.HasPrefix(v, "%[") {
			return v
		}
		n++
		return "%[" + strconv.Itoa(n) + "]" + v[1:]
	})
}

func wrapLine(line string, width int) []string {
	words := strings.Split(line, " ")
	var out []string
	cur := ""
	for _, w := range words {
		if cur != "" && len([]rune(cur))+1+len([]rune(w)) > width {
			out = append(out, cur)
			cur = w
			continue
		}
		if cur == "" {
			cur = w
		} else {
			cur += " " + w
		}
	}
	return append(out, cur)
}

// visual returns s (logical order, RTL base direction) in left-to-right display order.
func visual(s string) string {
	return visualWrap(s, rtlWrap)
}

// visualWrap - visual with lines broken at width runes (0: never, for one-line widgets
// like menu items, which show a line break as a box).
func visualWrap(s string, width int) string {
	if !strings.ContainsFunc(s, isRTL) {
		return s
	}
	s = indexVerbs(s)
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if width > 0 && len([]rune(l)) > width {
			lines = append(lines, wrapLine(l, width)...)
		} else {
			lines = append(lines, l)
		}
	}
	for i, l := range lines {
		lines[i] = visualLine(l)
	}
	return strings.Join(lines, "\n")
}

type bidiSeg struct {
	ltr  bool
	text []rune
}

func visualLine(line string) string {
	// fmt verbs are atomic LTR pieces
	var segs []bidiSeg
	locs := fmtVerb.FindAllStringIndex(line, -1)
	pos := 0
	add := func(part string) {
		rs := []rune(part)
		for i := 0; i < len(rs); {
			if !isLTR(rs[i]) {
				segs = append(segs, bidiSeg{false, rs[i : i+1]})
				i++
				continue
			}
			// LTR run: up to the last LTR rune before the next RTL rune
			j, last := i, i
			for j < len(rs) && !isRTL(rs[j]) {
				if isLTR(rs[j]) {
					last = j
				}
				j++
			}
			segs = append(segs, bidiSeg{true, rs[i : last+1]})
			i = last + 1
		}
	}
	for _, l := range locs {
		add(line[pos:l[0]])
		segs = append(segs, bidiSeg{true, []rune(line[l[0]:l[1]])})
		pos = l[1]
	}
	add(line[pos:])

	// neutrals between two LTR pieces (e.g. "2.5K / 720p", "%d – %d") join them
	var merged []bidiSeg
	for i := 0; i < len(segs); i++ {
		if segs[i].ltr {
			j := i + 1
			for j < len(segs) && !segs[j].ltr && !strings.ContainsFunc(string(segs[j].text), isRTL) {
				j++
			}
			if j < len(segs) && segs[j].ltr && j > i+1 {
				var text []rune
				for k := i; k <= j; k++ {
					text = append(text, segs[k].text...)
				}
				segs[j] = bidiSeg{true, text}
				continue
			}
		}
		merged = append(merged, segs[i])
	}

	var b []rune
	for i := len(merged) - 1; i >= 0; i-- {
		sg := merged[i]
		if sg.ltr {
			b = append(b, sg.text...)
			continue
		}
		for k := len(sg.text) - 1; k >= 0; k-- {
			r := sg.text[k]
			if m, ok := mirror[r]; ok {
				r = m
			}
			b = append(b, r)
		}
	}
	// Fyne's shaper still runs the bidi algorithm in an LTR paragraph: it flips each RTL
	// run (with the spaces, digits and punctuation it pulls in) and keeps the runs in
	// order. An LRM around every Hebrew word stops it pulling in anything else, so the
	// words alone get flipped, and they go back to logical order here.
	const lrm = '‎'
	var out []rune
	for i := 0; i < len(b); {
		if !isRTL(b[i]) {
			out = append(out, b[i])
			i++
			continue
		}
		j := i
		for j < len(b) && isRTL(b[j]) {
			j++
		}
		word := slices.Clone(b[i:j])
		slices.Reverse(word)
		out = append(append(append(out, lrm), word...), lrm)
		i = j
	}
	return string(out)
}

// rtlAlign right-aligns a wrapped label in Hebrew.
func rtlAlign(o any) {
	if l, ok := o.(*widget.Label); ok && lang == "he" {
		l.Alignment = fyne.TextAlignTrailing
	}
}
