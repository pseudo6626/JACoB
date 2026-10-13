#!/usr/bin/env bash
set -Eeuo pipefail

REPO="pseudo6626/JACoB"
ELITE_APP_ID="359320"
INSTALL_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/JACoB"
BIN_PATH="$INSTALL_DIR/JACoB"
LAUNCHER_PATH="$INSTALL_DIR/jacob-elite-launch"
JACOB_URL="http://127.0.0.1:6626/"
STEAM_RESTARTED=0
TMP_DIR=""

say() { printf '%s\n' "$*"; }
fail() { printf '\nJACoB Steam Deck setup failed: %s\n' "$*" >&2; exit 1; }
cleanup() {
  if [[ -n "${TMP_DIR:-}" && -d "$TMP_DIR" ]]; then
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT

need() {
  command -v "$1" >/dev/null 2>&1 || fail "Required command '$1' was not found."
}

find_steam_root() {
  local candidate
  for candidate in \
    "$HOME/.local/share/Steam" \
    "$HOME/.steam/steam" \
    "$HOME/.steam/root"; do
    if [[ -d "$candidate/userdata" ]]; then
      (cd "$candidate" && pwd -P)
      return 0
    fi
  done
  return 1
}

select_localconfig() {
  local root="$1"
  local -a configs=()
  local -a matches=()
  local file

  while IFS= read -r -d '' file; do
    configs+=("$file")
    if grep -Eq '^[[:space:]]*"359320"[[:space:]]*$' "$file"; then
      matches+=("$file")
    fi
  done < <(find "$root/userdata" -mindepth 3 -maxdepth 3 -type f -path '*/config/localconfig.vdf' -print0 2>/dev/null)

  if (( ${#matches[@]} == 1 )); then
    printf '%s\n' "${matches[0]}"
    return 0
  fi
  if (( ${#matches[@]} > 1 )); then
    ls -1t -- "${matches[@]}" 2>/dev/null | head -n1
    return 0
  fi
  if (( ${#configs[@]} == 1 )); then
    printf '%s\n' "${configs[0]}"
    return 0
  fi
  if (( ${#configs[@]} > 1 )); then
    ls -1t -- "${configs[@]}" 2>/dev/null | head -n1
    return 0
  fi
  return 1
}

stop_steam() {
  if ! pgrep -x steam >/dev/null 2>&1 && ! pgrep -f '/steamwebhelper' >/dev/null 2>&1; then
    return 0
  fi

  say "Closing Steam so its settings can be changed safely..."
  if command -v steam >/dev/null 2>&1; then
    steam -shutdown >/dev/null 2>&1 || true
  fi

  local i
  for i in {1..60}; do
    if ! pgrep -x steam >/dev/null 2>&1 && ! pgrep -f '/steamwebhelper' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  fail "Steam did not close. Close Steam completely and run this installer again."
}

start_steam() {
  if command -v steam >/dev/null 2>&1; then
    say "Restarting Steam..."
    nohup steam -silent >/dev/null 2>&1 &
    STEAM_RESTARTED=1
  fi
}

extract_launch_options() {
  local file="$1"
  awk -v appid="$ELITE_APP_ID" '
    function brace_delta(s,   i,c,d,q,esc) {
      d=0; q=0; esc=0
      for (i=1; i<=length(s); i++) {
        c=substr(s,i,1)
        if (q) {
          if (esc) { esc=0; continue }
          if (c=="\\") { esc=1; continue }
          if (c=="\"") q=0
          continue
        }
        if (c=="\"") { q=1; continue }
        if (c=="{") d++
        else if (c=="}") d--
      }
      return d
    }
    BEGIN { pending=0; inapp=0; depth=0 }
    {
      line=$0
      if (!inapp && line ~ "^[[:space:]]*\\\"" appid "\\\"[[:space:]]*$") {
        pending=1
        next
      }
      if (pending) {
        if (line ~ /^[[:space:]]*\{[[:space:]]*$/) {
          inapp=1
          depth=1
          pending=0
          next
        }
        if (line !~ /^[[:space:]]*$/) pending=0
      }
      if (inapp) {
        if (depth==1 && line ~ /^[[:space:]]*"LaunchOptions"[[:space:]]*"/) {
          value=line
          sub(/^[[:space:]]*"LaunchOptions"[[:space:]]*"/, "", value)
          sub(/"[[:space:]]*$/, "", value)
          print value
          exit
        }
        depth += brace_delta(line)
        if (depth <= 0) exit
      }
    }
  ' "$file"
}

patch_launch_options() {
  local file="$1"
  local new_options="$2"
  local out="$3"

  JACOB_REPLACEMENT="$new_options" awk -v appid="$ELITE_APP_ID" '
    function leading_ws(s) {
      match(s, /^[[:space:]]*/)
      return substr(s, 1, RLENGTH)
    }
    function brace_delta(s,   i,c,d,q,esc) {
      d=0; q=0; esc=0
      for (i=1; i<=length(s); i++) {
        c=substr(s,i,1)
        if (q) {
          if (esc) { esc=0; continue }
          if (c=="\\") { esc=1; continue }
          if (c=="\"") q=0
          continue
        }
        if (c=="\"") { q=1; continue }
        if (c=="{") d++
        else if (c=="}") d--
      }
      return d
    }
    BEGIN { replacement=ENVIRON["JACOB_REPLACEMENT"]; pending=0; inapp=0; depth=0; found_app=0; replaced=0; indent="" }
    {
      line=$0

      if (!inapp && line ~ "^[[:space:]]*\\\"" appid "\\\"[[:space:]]*$") {
        pending=1
        found_app=1
        print line
        next
      }

      if (pending) {
        print line
        if (line ~ /^[[:space:]]*\{[[:space:]]*$/) {
          inapp=1
          depth=1
          indent=leading_ws(line) "\t"
          pending=0
          next
        }
        if (line !~ /^[[:space:]]*$/) pending=0
        next
      }

      if (inapp) {
        delta=brace_delta(line)

        if (depth==1 && line ~ /^[[:space:]]*"LaunchOptions"[[:space:]]*"/) {
          printf "%s\"LaunchOptions\"\t\t\"%s\"\n", leading_ws(line), replacement
          replaced=1
        } else if (depth==1 && delta < 0) {
          if (!replaced) {
            printf "%s\"LaunchOptions\"\t\t\"%s\"\n", indent, replacement
            replaced=1
          }
          print line
        } else {
          print line
        }

        depth += delta
        if (depth <= 0) inapp=0
        next
      }

      print line
    }
    END {
      if (!found_app) exit 42
      if (!replaced) exit 43
    }
  ' "$file" > "$out"
}

say "JACoB Steam Deck Setup"
say "======================="
say ""
say "This installs the native Linux build of JACoB and wires Elite Dangerous"
say "to start JACoB automatically when the game launches."
say ""

need curl
need awk
need grep
need sed
need sha256sum
need find
need pgrep
need install

[[ "$(uname -m)" == "x86_64" ]] || fail "This installer currently supports x86_64/AMD64 Steam Deck and Linux systems only."
[[ "$EUID" -ne 0 ]] || fail "Run this installer as the normal Steam Deck user, not with sudo/root."

STEAM_ROOT="$(find_steam_root)" || fail "Steam's userdata directory was not found. Run this from Steam Deck Desktop Mode after Steam has been started at least once."
LOCALCONFIG="$(select_localconfig "$STEAM_ROOT")" || fail "Steam localconfig.vdf was not found."

TMP_DIR="$(mktemp -d)"
RELEASE_JSON="$TMP_DIR/releases.json"

say "Finding the newest published JACoB release..."
curl -fL --retry 3 --connect-timeout 10 \
  -H 'Accept: application/vnd.github+json' \
  "https://api.github.com/repos/$REPO/releases?per_page=20" \
  -o "$RELEASE_JSON"

TAG="$(grep -m1 -oE '"tag_name"[[:space:]]*:[[:space:]]*"v[^"]+"' "$RELEASE_JSON" | sed -E 's/.*"(v[^"]+)"/\1/')"
[[ -n "$TAG" ]] || fail "Could not determine the latest published JACoB release."
VERSION="${TAG#v}"
ASSET="JACoB-${VERSION}-linux-amd64"
BASE_URL="https://github.com/$REPO/releases/download/$TAG"

say "Downloading JACoB $VERSION..."
curl -fL --retry 3 --connect-timeout 10 "$BASE_URL/$ASSET" -o "$TMP_DIR/$ASSET"
curl -fL --retry 3 --connect-timeout 10 "$BASE_URL/SHA256SUMS.txt" -o "$TMP_DIR/SHA256SUMS.txt"

EXPECTED="$(awk -v asset="$ASSET" '$2 == asset {print $1; exit}' "$TMP_DIR/SHA256SUMS.txt")"
[[ "$EXPECTED" =~ ^[0-9a-fA-F]{64}$ ]] || fail "The release did not publish a SHA-256 digest for $ASSET."
ACTUAL="$(sha256sum "$TMP_DIR/$ASSET" | awk '{print $1}')"
[[ "${ACTUAL,,}" == "${EXPECTED,,}" ]] || fail "SHA-256 verification failed. The downloaded JACoB binary was not installed."

mkdir -p "$INSTALL_DIR"
if [[ -f "$BIN_PATH" ]]; then
  cp -p "$BIN_PATH" "$BIN_PATH.previous" 2>/dev/null || true
fi
install -m 0755 "$TMP_DIR/$ASSET" "$BIN_PATH.new"
mv -f "$BIN_PATH.new" "$BIN_PATH"

cat > "$LAUNCHER_PATH" <<EOF_LAUNCHER
#!/usr/bin/env bash
set -u

JACOB_BIN="$BIN_PATH"
JACOB_PID=""

if [[ -x "\$JACOB_BIN" ]]; then
  JACOB_OPEN_BROWSER=1 "\$JACOB_BIN" >/dev/null 2>&1 &
  JACOB_PID=\$!
fi

cleanup_jacob() {
  if [[ -n "\${JACOB_PID:-}" ]] && kill -0 "\$JACOB_PID" >/dev/null 2>&1; then
    kill "\$JACOB_PID" >/dev/null 2>&1 || true
    wait "\$JACOB_PID" 2>/dev/null || true
  fi
}
trap cleanup_jacob EXIT INT TERM

"\$@"
STATUS=\$?
exit \$STATUS
EOF_LAUNCHER
chmod 0755 "$LAUNCHER_PATH"

stop_steam

# Steam may have rewritten localconfig.vdf while shutting down, so resolve it again.
LOCALCONFIG="$(select_localconfig "$STEAM_ROOT")" || fail "Steam localconfig.vdf disappeared after Steam closed."
CURRENT_OPTIONS="$(extract_launch_options "$LOCALCONFIG" || true)"
LAUNCHER_MARKER="$LAUNCHER_PATH"

if [[ "$CURRENT_OPTIONS" == *"$LAUNCHER_MARKER"* ]]; then
  NEW_OPTIONS="$CURRENT_OPTIONS"
elif [[ "$CURRENT_OPTIONS" == *"%command%"* ]]; then
  NEW_OPTIONS="${CURRENT_OPTIONS/\%command%/$LAUNCHER_PATH %command%}"
elif [[ -n "$CURRENT_OPTIONS" ]]; then
  NEW_OPTIONS="$LAUNCHER_PATH %command% $CURRENT_OPTIONS"
else
  NEW_OPTIONS="$LAUNCHER_PATH %command%"
fi

BACKUP="$LOCALCONFIG.jacob-backup-$(date +%Y%m%d-%H%M%S)"
cp -p "$LOCALCONFIG" "$BACKUP"
PATCHED="$TMP_DIR/localconfig.vdf"
set +e
patch_launch_options "$LOCALCONFIG" "$NEW_OPTIONS" "$PATCHED"
PATCH_STATUS=$?
set -e
if [[ $PATCH_STATUS -eq 42 ]]; then
  fail "Elite Dangerous does not have a settings block in this Steam account yet. Launch Elite once, close it, then run this installer again. A backup was left at $BACKUP"
elif [[ $PATCH_STATUS -ne 0 ]]; then
  fail "Could not update Elite Dangerous launch options. Your original Steam config is backed up at $BACKUP"
fi
mv "$PATCHED" "$LOCALCONFIG"

start_steam

say ""
say "Installed successfully."
say ""
say "JACoB:        $BIN_PATH"
say "Elite hook:   $LAUNCHER_PATH"
say "Steam backup: $BACKUP"
say ""
say "Launch Elite Dangerous normally from Gaming Mode. JACoB will start with it"
say "and open its browser UI as the companion window."
