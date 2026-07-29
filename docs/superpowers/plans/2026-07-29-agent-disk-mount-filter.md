# Agent Disk Mount Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Report only the root filesystem and meaningful local or network data mounts, while excluding boot and system pseudo mounts so Agent collection succeeds under systemd sandboxing.

**Architecture:** Keep the existing filesystem enumeration through `gopsutil`. Add a mountpoint predicate for the `/boot` hierarchy and extend the filesystem-type denylist for pseudo filesystems, then apply both predicates before `diskUsage` is called. The protocol and dashboard payload remain unchanged.

**Tech Stack:** Go 1.26, gopsutil v4, Go standard-library tests, Docker Compose, GitHub Actions/GHCR.

---

### Task 1: Define the desired reportable mount set with a failing test

**Files:**
- Modify: `internal/agent/gopsutil_source_test.go:70-106`

- [ ] **Step 1: Expand `TestGopsutilSourceFiltersPersistentDisks` with boot, data, and pseudo mounts**

  Replace the test partition fixture with:

  ```go
  return []disk.PartitionStat{
      {Mountpoint: "/", Fstype: "ext4"},
      {Mountpoint: "/data", Fstype: "xfs"},
      {Mountpoint: "/net/nfs", Fstype: "nfs"},
      {Mountpoint: "/net/cifs", Fstype: "cifs"},
      {Mountpoint: "/net/sshfs", Fstype: "fuse.sshfs"},
      {Mountpoint: "/boot", Fstype: "ext4"},
      {Mountpoint: "/boot/efi", Fstype: "vfat"},
      {Mountpoint: "/boot/firmware", Fstype: "vfat"},
      {Mountpoint: "/bootdata", Fstype: "ext4"},
      {Mountpoint: "/run/credentials/unit", Fstype: "ramfs"},
      {Mountpoint: "/sys/kernel/debug", Fstype: "debugfs"},
      {Mountpoint: "/run", Fstype: "tmpfs"},
      {Mountpoint: "/container", Fstype: "overlay"},
      {Mountpoint: "/sys/fs/cgroup", Fstype: "cgroup2"},
  }, nil
  ```

  Make `diskUsage` append its mountpoint to `usageRequests`. Assert both collected disk mountpoints and usage requests equal:

  ```go
  "/,/data,/net/nfs,/net/cifs,/net/sshfs,/bootdata"
  ```

  In `TestPersistentFilesystemFilter`, add `ramfs`, `debugfs`, `securityfs`, `tracefs`, `devpts`, `nsfs`, and `pstore` to the false-case table. Add a dedicated assertion:

  ```go
  if !shouldReportFilesystem("/bootdata", "ext4") {
      t.Fatal("/bootdata must remain reportable")
  }
  for _, mountpoint := range []string{"/boot", "/boot/efi", "/boot/firmware"} {
      if shouldReportFilesystem(mountpoint, "ext4") {
          t.Errorf("shouldReportFilesystem(%q, ext4) = true, want false", mountpoint)
      }
  }
  ```

- [ ] **Step 2: Run the focused test and verify red**

  Run:

  ```bash
  go test ./internal/agent -run '^TestGopsutilSourceFiltersPersistentDisks$' -count=1
  ```

  Expected: FAIL because boot mounts and `ramfs` are currently included.

### Task 2: Filter boot hierarchy and pseudo filesystems before capacity collection

**Files:**
- Modify: `internal/agent/gopsutil_source.go:144-161`
- Modify: `internal/agent/gopsutil_source.go:235-244`

- [ ] **Step 1: Add the combined collection predicate**

  Add the helper:

  ```go
  func shouldReportFilesystem(mountpoint, filesystem string) bool {
      return isPersistentFilesystem(filesystem) && !isBootMount(mountpoint)
  }

  func isBootMount(mountpoint string) bool {
      return mountpoint == "/boot" || strings.HasPrefix(mountpoint, "/boot/")
  }
  ```

  In `PersistentDisks`, replace:

  ```go
  if !isPersistentFilesystem(partition.Fstype) {
      continue
  }
  ```

  with:

  ```go
  if !shouldReportFilesystem(partition.Mountpoint, partition.Fstype) {
      continue
  }
  ```

- [ ] **Step 2: Extend the pseudo-filesystem denylist**

  Add these entries to `temporaryFilesystems`:

  ```go
  "autofs": {}, "bpf": {}, "binfmt_misc": {}, "configfs": {},
  "debugfs": {}, "devpts": {}, "fusectl": {}, "hugetlbfs": {},
  "mqueue": {}, "nsfs": {}, "pstore": {}, "ramfs": {},
  "securityfs": {}, "tracefs": {},
  ```

  Do not add `vfat`, `nfs`, `cifs`, or `fuse.sshfs`: boot mounts are excluded by mountpoint, while data mounts using those types must remain reportable.

- [ ] **Step 3: Run the focused test and verify green**

  Run:

  ```bash
  go test ./internal/agent -run '^TestGopsutilSourceFiltersPersistentDisks$' -count=1
  ```

  Expected: PASS.

- [ ] **Step 4: Run the package test suite**

  Run:

  ```bash
  go test ./internal/agent -count=1
  ```

  Expected: PASS.

- [ ] **Step 5: Commit the production fix**

  ```bash
  git add internal/agent/gopsutil_source.go internal/agent/gopsutil_source_test.go
  git commit -m "fix: filter boot and pseudo filesystem mounts"
  ```

### Task 3: Verify the repository and publish the fixed Agent

**Files:**
- No source changes required.

- [ ] **Step 1: Run the full repository test suite**

  Run:

  ```bash
  go test ./...
  ```

  Expected: PASS.

- [ ] **Step 2: Build the production image locally**

  Run:

  ```bash
  sudo docker build -f deploy/Dockerfile -t 77probe:disk-filter .
  ```

  Expected: successful multi-stage build, including the Agent binaries in `/app/downloads/`.

- [ ] **Step 3: Push the main branch and wait for GHCR publication**

  Run:

  ```bash
  git push origin main
  ```

  Expected: GitHub Actions publishes `ghcr.io/zxxx98/77probe:latest` and the commit SHA tag.

- [ ] **Step 4: Deploy the published image on the TinyProbe host**

  After GitHub Actions succeeds, run from `/home/ubuntu/code/personal/77probe`:

  ```bash
  sudo docker compose pull tinyprobe
  sudo docker compose up -d tinyprobe
  curl -fsS http://127.0.0.1:23333/api/health
  ```

  Expected: `{"status":"ok"}`.

- [ ] **Step 5: Replace the remote Agent binary and restart it**

  On `ser113771339517`, run:

  ```bash
  sudo curl --fail --location --silent --show-error \
    --output /tmp/tinyprobe-agent \
    http://158.178.243.20:23333/downloads/tinyprobe-agent-linux-amd64
  sudo install -m 0755 /tmp/tinyprobe-agent /usr/local/bin/tinyprobe-agent
  sudo rm -f /tmp/tinyprobe-agent
  sudo systemctl restart tinyprobe-agent
  sudo systemctl is-active tinyprobe-agent
  ```

  Expected: `active`; the dashboard reports the server online within 10 seconds and lists `/` but not `/boot/efi`.
