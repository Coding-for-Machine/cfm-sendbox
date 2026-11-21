#!/usr/bin/env bash
# Minimal local test: no repo-builder image, no source builds.
# Creates a tiny "bash" package (uses the bash already inside the API image),
# serves it as a local package repo on :8000 and prints the next steps.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT=.quick/repo
rm -rf .quick && mkdir -p "$OUT" .quick/pkg
LANG_NAME=bash
VER=5.2.15
FILE="$LANG_NAME-$VER.pkg.tar.gz"
HOSTURL=${QUICK_HOST_URL:-http://host.docker.internal:8000}

cat > .quick/pkg/pkg-info.json <<JSON
{"language":"$LANG_NAME","version":"$VER","aliases":["sh","shell"],"build_platform":"docker-debian"}
JSON
cat > .quick/pkg/environment <<'ENVF'
#!/usr/bin/env bash
export PATH=$PWD/bin:$PATH
ENVF
cat > .quick/pkg/run <<'RUNF'
#!/usr/bin/env bash
# $1 is the main file, the rest are program arguments
bash "$@"
RUNF
printf 'echo OK\n' > .quick/pkg/test.bash
chmod +x .quick/pkg/run .quick/pkg/environment
tar -C .quick/pkg -czf "$OUT/$FILE" .

if command -v sha256sum >/dev/null; then SUM=$(sha256sum "$OUT/$FILE" | cut -d' ' -f1)
else SUM=$(shasum -a 256 "$OUT/$FILE" | cut -d' ' -f1); fi
printf '%s,%s,%s,%s/%s\n' "$LANG_NAME" "$VER" "$SUM" "$HOSTURL" "$FILE" > "$OUT/index"
echo "package: $OUT/$FILE"
echo "index:   $(cat "$OUT/index")"

cat <<MSG

Next (3 terminals / steps):
  1) API:   CFM_REPO_URL=$HOSTURL/index docker compose -f docker-compose.yaml up -d --build
  2) Repo:  this script now serves $OUT on :8000 (Ctrl+C to stop)
  3) Test:  ./cfm ppman install bash
            ./cfm run bash .quick/pkg/test.bash
MSG

if command -v go >/dev/null; then
    exec go run ./cmd/sendbox-repo -addr :8000 -dir "$OUT"
else
    cd "$OUT" && exec python3 -m http.server 8000
fi
