#!/usr/bin/env bash
# Hostile FSX filesystem matrix harness — loopback mounts for xfs, btrfs, vfat
# alongside a private host-FS work dir, runs the permanent hostile suite, unmounts.
#
# Usage:
#   integration/hostile/fsx/scripts/fsx-mount-matrix.sh [go-test-args...]
#
# Requires: sudo (passwordless preferred), mkfs.xfs, mkfs.btrfs, mkfs.vfat, go 1.26.5+
# Promotion-compatible: also exports E1_FS_ROOTS for the E1 evidence spike.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
WORK="${FOUNDRY_FSX_WORK_DIR:-/tmp/hostile-fsx-matrix-$$}"
IMG_DIR="$WORK/images"
MNT_DIR="$WORK/mnt"
ARTIFACT_DIR="${FOUNDRY_HOSTILE_FSX_ARTIFACT_DIR:-$ROOT/docs/evidence/hostile-fsx-logs}"

mkdir -p "$IMG_DIR" "$MNT_DIR" "$ARTIFACT_DIR"
chmod 0700 "$WORK" "$IMG_DIR" "$MNT_DIR" 2>/dev/null || true
find "$WORK" -type d -exec chmod 0700 {} + 2>/dev/null || true

cleanup() {
  set +e
  for m in "$MNT_DIR"/*; do
    if mountpoint -q "$m" 2>/dev/null; then
      sudo umount "$m" 2>/dev/null || sudo umount -l "$m" 2>/dev/null
    fi
  done
  rm -rf "$WORK"
}
trap cleanup EXIT

echo "hostile-fsx matrix work dir: $WORK"
echo "host: $(uname -a)"
echo "go: $(go version)"

HOST_DIR="$MNT_DIR/host"
mkdir -p "$HOST_DIR"
chmod -R 0700 "$WORK"
HOST_FS="$(findmnt -n -o FSTYPE -T "$HOST_DIR" 2>/dev/null || echo unknown)"
echo "host_fs=$HOST_FS path=$HOST_DIR"

XFS_IMG="$IMG_DIR/xfs.img"
XFS_MNT="$MNT_DIR/xfs"
dd if=/dev/zero of="$XFS_IMG" bs=1M count=320 status=none
mkfs.xfs -q "$XFS_IMG"
mkdir -p "$XFS_MNT"
sudo mount -o loop "$XFS_IMG" "$XFS_MNT"
sudo chown "$(id -u):$(id -g)" "$XFS_MNT"
chmod 0700 "$XFS_MNT"
echo "xfs mounted at $XFS_MNT"

BTRFS_IMG="$IMG_DIR/btrfs.img"
BTRFS_MNT="$MNT_DIR/btrfs"
dd if=/dev/zero of="$BTRFS_IMG" bs=1M count=128 status=none
mkfs.btrfs -q "$BTRFS_IMG"
mkdir -p "$BTRFS_MNT"
sudo mount -o loop "$BTRFS_IMG" "$BTRFS_MNT"
sudo chown "$(id -u):$(id -g)" "$BTRFS_MNT"
chmod 0700 "$BTRFS_MNT"
echo "btrfs mounted at $BTRFS_MNT"

VFAT_IMG="$IMG_DIR/vfat.img"
VFAT_MNT="$MNT_DIR/vfat"
dd if=/dev/zero of="$VFAT_IMG" bs=1M count=32 status=none
mkfs.vfat "$VFAT_IMG" >/dev/null
mkdir -p "$VFAT_MNT"
sudo mount -o "loop,uid=$(id -u),gid=$(id -g),umask=077" "$VFAT_IMG" "$VFAT_MNT"
echo "vfat mounted at $VFAT_MNT"

export FOUNDRY_FSX_ROOTS="${HOST_FS}:${HOST_DIR},xfs:${XFS_MNT},btrfs:${BTRFS_MNT},vfat:${VFAT_MNT}"
export E1_FS_ROOTS="$FOUNDRY_FSX_ROOTS"
export FOUNDRY_HOSTILE_FSX_ARTIFACT_DIR="$ARTIFACT_DIR"
echo "FOUNDRY_FSX_ROOTS=$FOUNDRY_FSX_ROOTS"

cd "$ROOT"
set +e
go test -tags=hostile -count=1 -timeout 300s -v ./integration/hostile/fsx/ "$@"
suite_rc=$?
go test -count=1 -timeout 120s ./integration/hostile/e1/
e1_rc=$?
set -e

echo "suite_rc=$suite_rc e1_rc=$e1_rc"
if [[ "$suite_rc" -ne 0 || "$e1_rc" -ne 0 ]]; then
  exit 1
fi
