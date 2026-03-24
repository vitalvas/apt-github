#!/bin/sh
set -e

tmpfile=$(mktemp /tmp/apt-github-XXXXXX.sources)
trap 'rm -f "$tmpfile"' EXIT

found=0
for file in /etc/apt/sources.list.d/*.sources /etc/apt/sources.list.d/*.list; do
  [ -f "$file" ] || continue
  if grep -q 'github://' "$file" 2>/dev/null; then
    cat "$file" >> "$tmpfile"
    printf '\n' >> "$tmpfile"
    found=1
  fi
done

if [ "$found" -eq 0 ]; then
  echo "No github sources found" >&2
  exit 0
fi

apt-get update \
  -o Dir::Etc::sourcelist="$tmpfile" \
  -o Dir::Etc::sourceparts=/dev/null \
  -o APT::Get::List-Cleanup=0

apt-get upgrade -y \
  -o Dir::Etc::sourcelist="$tmpfile" \
  -o Dir::Etc::sourceparts=/dev/null
