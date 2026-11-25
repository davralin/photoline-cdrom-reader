#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TMP_DIR=$(mktemp -d)
APP_PID=
cleanup() {
  if [ -n "$APP_PID" ]; then
    kill "$APP_PID" 2>/dev/null || true
    wait "$APP_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

export HOME="$TMP_DIR/home"
export GOCACHE="$TMP_DIR/go-cache"
mkdir -p "$HOME" "$TMP_DIR/2004/Pictures" "$ROOT/docs"
python3 "$ROOT/tools/generate-demo-data.py" "$TMP_DIR/2004"

magick -size 1200x680 gradient:'#b9e4ed-#fff0d3' \
  -fill '#f6c95e' -stroke none -draw 'circle 1010,105 1080,105' \
  -fill '#9bcdb6' -draw 'polygon 0,475 210,325 440,475 710,340 980,475 1200,360 1200,680 0,680' \
  -fill '#7caf97' -draw 'polygon 0,555 270,415 520,565 845,410 1200,555 1200,680 0,680' \
  -stroke '#34535b' -strokewidth 8 -fill '#f4d0aa' \
  -draw 'circle 250,310 282,310 circle 480,285 512,285 circle 705,310 737,310 circle 930,285 962,285' \
  -stroke none -fill '#d16c58' -draw 'roundrectangle 215,348 285,510 18,18' \
  -fill '#457b9d' -draw 'roundrectangle 445,323 515,510 18,18' \
  -fill '#e9a23b' -draw 'roundrectangle 670,348 740,510 18,18' \
  -fill '#7b6aa8' -draw 'roundrectangle 895,323 965,510 18,18' \
  -stroke '#34535b' -strokewidth 8 -draw 'line 230,500 210,610 line 270,500 292,610 line 460,500 440,610 line 500,500 522,610 line 685,500 665,610 line 725,500 748,610 line 910,500 890,610 line 950,500 972,610' \
  -stroke none -fill '#ffffffcc' -draw 'roundrectangle 28,24 520,94 14,14' \
  -font DejaVu-Sans-Bold -pointsize 30 -fill '#203943' -annotate +48+70 'SYNTHETIC SAMPLE  ·  NOT FROM THE DISC' \
  "$TMP_DIR/2004/Pictures/demo-class-5a.jpg"

cd "$ROOT"
CGO_ENABLED=0 go build -buildvcs=false -trimpath -o "$TMP_DIR/reader" .
chmod 755 "$TMP_DIR/reader"
"$TMP_DIR/reader" -data "$TMP_DIR/2004" -addr 127.0.0.1:18080 -no-browser >"$TMP_DIR/reader.log" 2>&1 &
APP_PID=$!

ready=false
for _ in $(seq 1 60); do
  if curl --fail --silent http://127.0.0.1:18080/health >/dev/null; then
    ready=true
    break
  fi
  sleep 0.25
done
if [ "$ready" != true ]; then
  printf '%s\n' 'Synthetic viewer did not become ready.' >&2
  cat "$TMP_DIR/reader.log" >&2
  exit 1
fi

chromium --headless --no-sandbox --disable-gpu --disable-dev-shm-usage \
  --hide-scrollbars --user-data-dir="$TMP_DIR/chrome-profile" \
  --window-size=1600,1450 --virtual-time-budget=5000 \
  --screenshot="$ROOT/docs/screenshot-synthetic.png" \
  'http://127.0.0.1:18080/?class=0'
test -s "$ROOT/docs/screenshot-synthetic.png"
printf 'Synthetic screenshot written to %s\n' "$ROOT/docs/screenshot-synthetic.png"
