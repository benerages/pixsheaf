#!/bin/sh
# Builds the worker's runtime root filesystem (ADR 0018).
#
# Usage: make-rootfs <binary> <root>
#
# 1. Ask the dynamic loader which shared libraries the binary needs.
# 2. Install the Wolfi packages that own them into <root> with apk, at the
#    exact versions of this build stage, so scanners can read the apk
#    database. Install scripts are not run.
# 3. Copy the libraries built from source (/usr/local/lib) and the binary.
# 4. Verify: same package versions as the build stage, no shell, no package
#    manager, no libimagequant (ADR 0017), and the binary starts.
set -eu
bin=$1 root=$2

fail() {
  echo "make-rootfs: $*" >&2
  exit 1
}

ldso=""
for candidate in /usr/lib/ld-linux-*.so.* /lib/ld-linux-*.so.*; do
  if [ -e "$candidate" ]; then
    ldso=$candidate
    break
  fi
done
[ -n "$ldso" ] || fail "dynamic loader not found"

libs=$("$ldso" --list "$bin" | awk '$2 == "=>" { print $3; next } $1 ~ /^\// { print $1 }')
[ -n "$libs" ] || fail "no shared libraries found for $bin"

pkgs=""
for lib in $libs; do
  real=$(readlink -f "$lib")
  case "$real" in
    /usr/local/*) continue ;;
  esac
  owner=$(apk info --who-owns "$real" | awk '{ print $NF }')
  [ -n "$owner" ] || fail "no package owns $real"
  # name-1.2.3-r4 -> name=1.2.3-r4
  pkgs="$pkgs $(echo "$owner" | sed 's/-\([^-]*-r[0-9]*\)$/=\1/')"
done
pkgs=$(echo "$pkgs" | tr ' ' '\n' | sed '/^$/d' | sort -u)
echo "make-rootfs: runtime packages:"
echo "$pkgs"

mkdir -p "$root/etc/apk"
cp -a /etc/apk/keys "$root/etc/apk/"
cp /etc/apk/repositories "$root/etc/apk/"
# shellcheck disable=SC2086 # one package per word
apk add --root "$root" --initdb --no-cache --no-scripts $pkgs

apk info -v | sort >/tmp/build.pkgs
apk info --root "$root" -v | sort >/tmp/root.pkgs
drift=$(comm -23 /tmp/root.pkgs /tmp/build.pkgs)
[ -z "$drift" ] || fail "runtime packages differ from the build stage:
$drift"

mkdir -p "$root/usr/local/lib" "$root/usr/local/bin"
cp -a /usr/local/lib/lib*.so* "$root/usr/local/lib/"
cp "$bin" "$root/usr/local/bin/"

for forbidden in bin/sh bin/busybox bin/bash sbin/apk usr/bin/apk; do
  [ ! -e "$root/$forbidden" ] || fail "$forbidden must not be in the runtime image"
done
if find "$root" -name 'libimagequant*' | grep -q .; then
  fail "libimagequant must not be in the runtime image (ADR 0017)"
fi

out=$(env LD_LIBRARY_PATH=/usr/local/lib chroot "$root" "/usr/local/bin/$(basename "$bin")" version)
echo "make-rootfs: $out"
case "$out" in
  *"libvips 8."*) ;;
  *) fail "binary does not report libvips inside the runtime root" ;;
esac
