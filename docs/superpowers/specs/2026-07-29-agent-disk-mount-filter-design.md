# Agent disk mount filter

## Goal

Agent reports capacity only for the system filesystem and meaningful data mounts. It must not fail when system-managed pseudo mounts are inaccessible under the systemd sandbox.

## Scope

The Agent continues to enumerate mounted filesystems. A mount is reported only when both conditions hold:

1. Its filesystem type is not a temporary or system pseudo filesystem.
2. Its mountpoint is neither `/boot` nor any path below `/boot/`.

The root filesystem (`/`) and ordinary data mounts such as `/data`, `/mnt`, `/srv`, NFS, CIFS, and SSHFS remain eligible.

## Design

The existing filesystem-type predicate is extended to classify `ramfs` and the other system-only filesystems seen in normal Linux mount tables as non-reportable. A mountpoint predicate excludes the boot hierarchy without excluding similarly named paths such as `/bootdata`.

`PersistentDisks` applies the combined predicate before calling `diskUsage`. This prevents an inaccessible pseudo mount such as `/run/credentials/systemd-sysusers.service` from aborting the entire report collection.

## Tests

Tests will verify that:

- `/`, ordinary local data mounts, and NFS/CIFS/SSHFS mounts are reported.
- `/boot`, `/boot/efi`, and nested boot mounts are excluded.
- `ramfs` and existing temporary/system filesystems are excluded before capacity collection.
- A mountpoint outside the boot hierarchy whose name begins with `boot` remains eligible.

## Out of scope

This change does not add a physical disk inventory based on `lsblk`, alter aggregate disk I/O, or change the dashboard schema.
