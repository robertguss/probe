#!/usr/bin/env bash
# E1 filesystem matrix harness — creates loopback mounts for xfs, btrfs, vfat
# alongside a private ext4 (or host FS) work dir, runs the spike tests, unmounts.
#
# Usage:
#   integration/hostile/e1/scripts/e1-mount-matrix.sh [go-test-args...]
#
# Requires: sudo (passwordless preferred), mkfs.xfs, mkfs.btrfs, mkfs.vfat, go 1.26.5+
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
WORK="${E1_WORK_DIR:-/tmp/e1-matrix-$$}"
IMG_DIR="$WORK/images"
MNT_DIR="$WORK/mnt"
LOG_DIR="${E1_LOG_DIR:-$ROOT/docs/evidence/e1-logs}"

mkdir -p "$IMG_DIR" "$MNT_DIR" "$LOG_DIR"
# Entire harness tree must be 0700 so the parent walk custody check passes
# (group-writable non-sticky components fail fs.namespace_not_private).
chmod 0700 "$WORK" "$IMG_DIR" "$MNT_DIR" 2>/dev/null || true
# Also tighten any pre-existing umask-expanded parents under /tmp/e1-matrix-*.
find "$WORK" -type d -exec chmod 0700 {} + 2>/dev/null || true

cleanup() {
  set +e
  for m in "$MNT_DIR"/*; do
    if mountpoint -q "$m" 2>/dev/null; then
      sudo umount "$m" 2>/dev/null || sudo umount -l "$m" 2>/dev/null
    fi
  done
  # Images and empty mount points are harness scratch (not Foundry stages).
  rm -rf "$WORK"
}
trap cleanup EXIT

echo "E1 matrix work dir: $WORK"
echo "host: $(uname -a)"
echo "go: $(GOTOOLCHAIN=go1.26.5 go version)"

# --- host FS private dir (usually ext4) ---
HOST_DIR="$MNT_DIR/host"
mkdir -p "$HOST_DIR"
chmod -R 0700 "$WORK"
HOST_FS="$(findmnt -n -o FSTYPE -T "$HOST_DIR" 2>/dev/null || echo unknown)"
echo "host_fs=$HOST_FS path=$HOST_DIR"

# --- xfs (needs >= ~300MB) ---
XFS_IMG="$IMG_DIR/xfs.img"
XFS_MNT="$MNT_DIR/xfs"
dd if=/dev/zero of="$XFS_IMG" bs=1M count=320 status=none
mkfs.xfs -q "$XFS_IMG"
mkdir -p "$XFS_MNT"
sudo mount -o loop "$XFS_IMG" "$XFS_MNT"
sudo chown "$(id -u):$(id -g)" "$XFS_MNT"
chmod 0700 "$XFS_MNT"
echo "xfs mounted at $XFS_MNT"

# --- btrfs ---
BTRFS_IMG="$IMG_DIR/btrfs.img"
BTRFS_MNT="$MNT_DIR/btrfs"
dd if=/dev/zero of="$BTRFS_IMG" bs=1M count=128 status=none
mkfs.btrfs -q "$BTRFS_IMG"
mkdir -p "$BTRFS_MNT"
sudo mount -o loop "$BTRFS_IMG" "$BTRFS_MNT"
sudo chown "$(id -u):$(id -g)" "$BTRFS_MNT"
chmod 0700 "$BTRFS_MNT"
echo "btrfs mounted at $BTRFS_MNT"

# --- vfat (negative rename_unsupported probe) ---
VFAT_IMG="$IMG_DIR/vfat.img"
VFAT_MNT="$MNT_DIR/vfat"
dd if=/dev/zero of="$VFAT_IMG" bs=1M count=32 status=none
mkfs.vfat "$VFAT_IMG" >/dev/null
mkdir -p "$VFAT_MNT"
sudo mount -o "loop,uid=$(id -u),gid=$(id -g),umask=077" "$VFAT_IMG" "$VFAT_MNT"
echo "vfat mounted at $VFAT_MNT"

# Label host FS with its real type (ext4 expected).
export E1_FS_ROOTS="${HOST_FS}:${HOST_DIR},xfs:${XFS_MNT},btrfs:${BTRFS_MNT},vfat:${VFAT_MNT}"
echo "E1_FS_ROOTS=$E1_FS_ROOTS"

# Machine / kernel / versions snapshot
{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "uname: $(uname -a)"
  echo "os-release: $(. /etc/os-release 2>/dev/null; echo ${PRETTY_NAME:-unknown})"
  echo "go: $(GOTOOLCHAIN=go1.26.5 go version)"
  echo "x/sys: $(cd "$ROOT" && GOTOOLCHAIN=go1.26.5 go list -m golang.org/x/sys)"
  echo "E1_FS_ROOTS=$E1_FS_ROOTS"
  findmnt -T "$HOST_DIR" || true
  findmnt -T "$XFS_MNT" || true
  findmnt -T "$BTRFS_MNT" || true
  findmnt -T "$VFAT_MNT" || true
} | tee "$LOG_DIR/machine.txt"

# Cross-compile Darwin commit adapter (SV-02 symbol presence).
echo "cross-compile GOOS=darwin (commit_darwin.go / RenameatxNp)..."
(
  cd "$ROOT"
  GOTOOLCHAIN=go1.26.5 GOOS=darwin GOARCH=amd64 go test -c -o /dev/null ./integration/hostile/e1/
) 2>&1 | tee "$LOG_DIR/darwin-crosscompile.txt"

# Run tests
echo "running E1 tests..."
set +e
(
  cd "$ROOT"
  GOTOOLCHAIN=go1.26.5 go test ./integration/hostile/e1/ -count=1 -v -timeout 120s "$@"
) 2>&1 | tee "$LOG_DIR/go-test.txt"
TEST_EXIT=${PIPESTATUS[0]}
set -e

echo "test_exit=$TEST_EXIT"
exit "$TEST_EXIT"
