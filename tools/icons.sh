#!/bin/sh
# Builds the app icons (cmd/xiaomi-cam/icons/*.svg) from Lucide (https://lucide.dev, ISC):
# the strokes become filled outlines, because Fyne recolors only fills (theme colors,
# disabled state). Needs inkscape.
#   npm pack lucide-static@0.460.0 && tar xzf lucide-static-0.460.0.tgz
#   tools/icons.sh package/icons chevron-left chevron-right …
set -e
src=$1; shift
out=$(dirname "$0")/../cmd/xiaomi-cam/icons
mkdir -p "$out"
tmp=$(mktemp -d)
for name in "$@"; do
	sed 's/currentColor/#000000/g; /@license/d' "$src/$name.svg" > "$tmp/$name.svg"
	inkscape "$tmp/$name.svg" --actions="select-all:all;object-to-path;object-stroke-to-path" \
		--export-plain-svg --export-type=svg --export-filename="$tmp/$name.out.svg" 2>/dev/null
	# fills only: no fill="none" left on the root, every shape black
	python3 - "$tmp/$name.out.svg" "$out/$name.svg" <<'PY'
import re, sys
s = open(sys.argv[1]).read()
s = re.sub(r'<\?xml[^>]*>\s*', '', s)
s = re.sub(r'<metadata.*?</metadata>|<defs[^>]*/>|<defs.*?</defs>', '', s, flags=re.S)
s = re.sub(r'\s(class|id|style|sodipodi:\w+|inkscape:\w+|xmlns:\w+)="[^"]*"', '', s)
s = re.sub(r'\s(fill|stroke|stroke-width|stroke-linecap|stroke-linejoin)="[^"]*"', '', s)
s = re.sub(r'\s+', ' ', s).replace('> <', '><').strip()
s = re.sub(r'-?\d+\.\d{3,}', lambda m: ('%.2f' % float(m.group())).rstrip('0').rstrip('.'), s)  # smaller files
s = s.replace('<path ', '<path fill="#000000" ')
open(sys.argv[2], 'w').write(s + '\n')
PY
done
rm -rf "$tmp"
