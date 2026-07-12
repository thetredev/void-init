package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// nbdDevice is the network block device void-mkinitfs attaches images to.
// v1 hardcodes a single device rather than scanning for a free one, since
// void-mkinitfs is meant to run one build at a time.
const nbdDevice = "/dev/nbd0"

// layout identifies which of the two partition schemes an image uses, per
// void-mkinitfs.md step 2.
type layout int

const (
	layoutBIOS layout = iota
	layoutEFI
)

func (l layout) String() string {
	if l == layoutEFI {
		return "efi"
	}
	return "bios"
}

// partitionCount is how many partitions l's layout has on disk.
func (l layout) partitionCount() int {
	if l == layoutEFI {
		return 4
	}
	return 3
}

// createImage creates a fresh 3G qcow2 file at path.
func createImage(path string) error {
	logInfo("creating qcow2 image %s (3G)", path)
	_, err := runCommand("qemu-img", "create", "-f", "qcow2", path, "3G")
	return err
}

// attachImage connects path to nbdDevice via qemu-nbd, loading the nbd
// kernel module first if needed. The caller is responsible for pushing
// detachImage onto the cleanup stack immediately after a successful call.
func attachImage(path string) error {
	if _, err := runCommand("modprobe", "nbd"); err != nil {
		return err
	}

	logInfo("attaching %s to %s", path, nbdDevice)
	if _, err := runCommand("qemu-nbd", "-c", nbdDevice, path); err != nil {
		return err
	}

	return nil
}

// detachImage disconnects nbdDevice. Errors are logged, not returned,
// since this only ever runs from cleanup, where there's nothing left to
// propagate a failure to.
func detachImage() {
	logInfo("detaching %s", nbdDevice)
	if _, err := runCommand("qemu-nbd", "-d", nbdDevice); err != nil {
		logWarn("%v", err)
	}
}

// partitionSpec is one sgdisk partition definition: -n/-t/-c combined
// into a single invocation, mirroring void-mkinitfs.md step 2.
type partitionSpec struct {
	num  int
	size string // sgdisk end-of-partition spec, e.g. "+1M", or "0" for the rest of the disk
	typ  string // sgdisk hex type code
	name string
}

// partitionSpecs returns the sgdisk partition table for l.
func partitionSpecs(l layout) []partitionSpec {
	switch l {
	case layoutEFI:
		return []partitionSpec{
			{1, "+1M", "ef02", "BIOS boot"},
			{2, "+199M", "ef00", "EFI"},
			{3, "+300M", "8300", "boot"},
			{4, "0", "8300", "root"},
		}
	default:
		return []partitionSpec{
			{1, "+1M", "ef02", "BIOS boot"},
			{2, "+499M", "8300", "boot"},
			{3, "0", "8300", "root"},
		}
	}
}

// partitionDevice returns the /dev/nbd0pN device path for partition n.
func partitionDevice(n int) string {
	return fmt.Sprintf("%sp%d", nbdDevice, n)
}

// rootPartitionDevice returns the device node for l's root partition -
// always the last partition in the layout.
func rootPartitionDevice(l layout) string {
	return partitionDevice(l.partitionCount())
}

// bootPartitionDevice returns the device node for l's /boot partition -
// always the second-to-last partition in the layout.
func bootPartitionDevice(l layout) string {
	return partitionDevice(l.partitionCount() - 1)
}

// efiPartitionDevice returns the device node for the EFI System Partition.
// Only meaningful for layoutEFI.
func efiPartitionDevice() string {
	return partitionDevice(2)
}

// partition writes a fresh GPT partition table for l onto nbdDevice via
// sgdisk, then waits for the kernel to expose the resulting partition
// device nodes.
func partition(l layout) error {
	logInfo("partitioning %s for %s layout", nbdDevice, l)

	if _, err := runCommand("sgdisk", "-o", nbdDevice); err != nil {
		return err
	}

	for _, p := range partitionSpecs(l) {
		args := []string{
			"sgdisk",
			"-n", fmt.Sprintf("%d:0:%s", p.num, p.size),
			"-t", fmt.Sprintf("%d:%s", p.num, p.typ),
			"-c", fmt.Sprintf("%d:%s", p.num, p.name),
			nbdDevice,
		}
		if _, err := runCommand(args...); err != nil {
			return err
		}
	}

	if _, err := runCommand("partprobe", nbdDevice); err != nil {
		return err
	}

	return nil
}

// makeFilesystems formats every partition in l's layout with the
// filesystem described in void-mkinitfs.md step 3.
func makeFilesystems(l layout) error {
	if l == layoutEFI {
		if _, err := runCommand("mkfs.vfat", "-F32", "-n", "EFI", efiPartitionDevice()); err != nil {
			return err
		}
	}

	if _, err := runCommand("mkfs.ext2", "-L", "boot", bootPartitionDevice(l)); err != nil {
		return err
	}
	if _, err := runCommand("mkfs.ext4", "-L", "root", rootPartitionDevice(l)); err != nil {
		return err
	}

	return nil
}

// mountTarget holds the mounted root of the image being built.
type mountTarget struct {
	root string
}

// mount mounts l's partitions under a fresh temp directory: root first,
// then /boot (and, for EFI, /boot/efi) nested inside it. Each successful
// mount is pushed onto stack immediately, so a failure partway through
// still unwinds whatever did mount.
func mount(l layout, stack *cleanupStack) (*mountTarget, error) {
	root, err := os.MkdirTemp("", "void-mkinitfs-root")
	if err != nil {
		return nil, fmt.Errorf("create mount root: %w", err)
	}
	stack.push(func() {
		if err := os.RemoveAll(root); err != nil {
			logWarn("remove %s: %v", root, err)
		}
	})

	if err := mountAt(rootPartitionDevice(l), root, stack); err != nil {
		return nil, err
	}

	bootDir := filepath.Join(root, "boot")
	if err := os.Mkdir(bootDir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", bootDir, err)
	}
	if err := mountAt(bootPartitionDevice(l), bootDir, stack); err != nil {
		return nil, err
	}

	if l == layoutEFI {
		efiDir := filepath.Join(bootDir, "efi")
		if err := os.Mkdir(efiDir, 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", efiDir, err)
		}
		if err := mountAt(efiPartitionDevice(), efiDir, stack); err != nil {
			return nil, err
		}
	}

	return &mountTarget{root: root}, nil
}

// mountAt mounts dev at dir and pushes its unmount onto stack immediately
// on success.
func mountAt(dev, dir string, stack *cleanupStack) error {
	logInfo("mounting %s at %s", dev, dir)
	if _, err := runCommand("mount", dev, dir); err != nil {
		return err
	}

	stack.push(func() {
		logInfo("unmounting %s", dir)
		if _, err := runCommand("umount", dir); err != nil {
			logWarn("%v", err)
		}
	})

	return nil
}

// detectLayout infers whether the image already attached at nbdDevice
// uses the BIOS (3-partition) or EFI (4-partition) layout by counting the
// partition device nodes the kernel exposes, per void-mkinitfs.md step
// 10. It does not otherwise validate partition types, filesystem types,
// or sizes - pointing -i at an image with some other 3- or 4-partition
// scheme will mount the wrong partition at the wrong path silently.
func detectLayout() (layout, error) {
	if _, err := runCommand("partprobe", nbdDevice); err != nil {
		return 0, err
	}

	matches, err := filepath.Glob(nbdDevice + "p*")
	if err != nil {
		return 0, fmt.Errorf("glob %sp*: %w", nbdDevice, err)
	}
	sort.Strings(matches)

	switch len(matches) {
	case layoutBIOS.partitionCount():
		logInfo("found %d partitions on %s, assuming %s layout", len(matches), nbdDevice, layoutBIOS)
		return layoutBIOS, nil
	case layoutEFI.partitionCount():
		logInfo("found %d partitions on %s, assuming %s layout", len(matches), nbdDevice, layoutEFI)
		return layoutEFI, nil
	default:
		return 0, fmt.Errorf("found %d partitions on %s, expected %d (bios) or %d (efi) - refusing to guess",
			len(matches), nbdDevice, layoutBIOS.partitionCount(), layoutEFI.partitionCount())
	}
}
