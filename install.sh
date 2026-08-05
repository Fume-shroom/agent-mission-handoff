#!/bin/sh

set -eu

repo="Fume-shroom/agent-mission-handoff"
install_dir="${AMH_INSTALL_DIR:-$HOME/.local/bin}"
version="${AMH_VERSION:-latest}"

fail() {
  printf 'amh installer: %s\n' "$1" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"

case "$(uname -s)" in
  Darwin) os="Darwin" ;;
  Linux) os="Linux" ;;
  *) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="x86_64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) fail "unsupported CPU architecture: $(uname -m)" ;;
esac

asset="amh_${os}_${arch}.tar.gz"
if [ -n "${AMH_RELEASE_BASE:-}" ]; then
  release_base="$AMH_RELEASE_BASE"
elif [ "$version" = "latest" ]; then
  release_base="https://github.com/$repo/releases/latest/download"
else
  release_base="https://github.com/$repo/releases/download/$version"
fi

tmp_dir="$(mktemp -d 2>/dev/null || mktemp -d -t amh-install)"
install_tmp=""
cleanup() {
  if [ -n "$install_tmp" ]; then
    rm -f "$install_tmp"
  fi
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

printf 'Downloading %s...\n' "$asset"
curl -fsSL "$release_base/$asset" -o "$tmp_dir/$asset"
curl -fsSL "$release_base/checksums.txt" -o "$tmp_dir/checksums.txt"

expected="$(awk -v file="$asset" '$2 == file { print $1 }' "$tmp_dir/checksums.txt")"
[ -n "$expected" ] || fail "checksum for $asset was not published"

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp_dir/$asset" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$tmp_dir/$asset" | awk '{ print $1 }')"
else
  fail "sha256sum or shasum is required"
fi

[ "$actual" = "$expected" ] || fail "checksum verification failed"

mkdir -p "$tmp_dir/package"
tar -xzf "$tmp_dir/$asset" -C "$tmp_dir/package"
[ -x "$tmp_dir/package/amh" ] || fail "release archive does not contain amh"

mkdir -p "$install_dir"
install_tmp="$(mktemp "$install_dir/.amh.XXXXXX")"
cp "$tmp_dir/package/amh" "$install_tmp"
chmod 0755 "$install_tmp"
mv -f "$install_tmp" "$install_dir/amh"
install_tmp=""

skill_source="$tmp_dir/package/skills/mission-handoff/SKILL.md"
if [ -f "$skill_source" ]; then
  for agent_home in "$HOME/.codex" "$HOME/.claude"; do
    skill_dir="$agent_home/skills/mission-handoff"
    mkdir -p "$skill_dir"
    cp "$skill_source" "$skill_dir/SKILL.md"
    chmod 0644 "$skill_dir/SKILL.md"
  done
fi

case ":$PATH:" in
  *":$install_dir:"*) path_updated="false" ;;
  *)
    path_updated="true"
    shell_name="$(basename "${SHELL:-sh}")"
    case "$shell_name" in
      zsh) profile="$HOME/.zshrc" ;;
      bash)
        if [ "$(uname -s)" = "Darwin" ]; then
          profile="$HOME/.bash_profile"
        else
          profile="$HOME/.bashrc"
        fi
        ;;
      *) profile="$HOME/.profile" ;;
    esac
    path_line="export PATH=\"$install_dir:\$PATH\""
    touch "$profile"
    grep -F "$path_line" "$profile" >/dev/null 2>&1 || printf '\n%s\n' "$path_line" >> "$profile"
    ;;
esac

printf '\nInstalled amh to %s\n' "$install_dir/amh"
printf 'Installed the Mission Handoff Skill for Codex and Claude Code.\n'
"$install_dir/amh" version

if [ "$path_updated" = "true" ]; then
  printf 'PATH was updated. Open a new terminal before running amh directly.\n'
fi
