package filesystem

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
	ctrl "sigs.k8s.io/controller-runtime"
)

var utilLogger = ctrl.Log.WithName("util")

const (
	blkidCmd = "/sbin/blkid"
)

type temporaryer interface {
	Temporary() bool
}

func isSameDevice(dev1, dev2 string) (bool, error) {
	utilLogger.Info("isSameDevice")
	if dev1 == dev2 {
		return true, nil
	}

	var st1, st2 unix.Stat_t
	utilLogger.Info("exec Stat", "dev1", dev1)
	if err := Stat(dev1, &st1); err != nil {
		return false, fmt.Errorf("stat failed for %s: %v", dev1, err)
	}
	utilLogger.Info("exec Stat", "dev2", dev2)
	if err := Stat(dev2, &st2); err != nil {
		return false, fmt.Errorf("stat failed for %s: %v", dev2, err)
	}

	return st1.Rdev == st2.Rdev, nil
}

// IsMounted returns true if device is mounted on target.
// The implementation uses /proc/mounts because some filesystem uses a virtual device.
func IsMounted(device, target string) (bool, error) {
	utilLogger.Info("IsMounted", "device", device)

	abs, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}
	utilLogger.Info("EvalSymlinks1", "device", device, "path", abs)
	target, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return false, err
	}

	data, err := ioutil.ReadFile("/proc/mounts")
	if err != nil {
		return false, fmt.Errorf("could not read /proc/mounts: %v", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		utilLogger.Info("EvalSymlinks2", "device", device, "path", fields[1])
		d, err := filepath.EvalSymlinks(fields[1])
		if err != nil {
			return false, err
		}
		if d == target {
			return isSameDevice(device, fields[0])
		}
	}

	return false, nil
}

// DetectFilesystem returns filesystem type if device has a filesystem.
// This returns an empty string if no filesystem exists.
func DetectFilesystem(device string) (string, error) {
	utilLogger.Info("DetectFilesystem", "device", device)

	f, err := os.Open(device)
	if err != nil {
		return "", err
	}
	// synchronizes dirty data
	utilLogger.Info("Sync", "device", device)
	err = f.Sync()
	if err != nil {
		utilLogger.Info("Sync error", "device", device, "err", err)
	}
	f.Close()

	utilLogger.Info("exec blkid", "device", device)
	out, err := exec.Command(blkidCmd, "-c", "/dev/null", "-o", "export", device).CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// blkid exists with status 2 when anything can be found
			if exitErr.ExitCode() == 2 {
				return "", nil
			}
		}
		return "", fmt.Errorf("blkid failed: output=%s, device=%s, error=%v", string(out), device, err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "TYPE=") {
			return line[5:], nil
		}
	}

	return "", nil
}

// Stat wrapped a golang.org/x/sys/unix.Stat function to handle EINTR signal for Go 1.14+
func Stat(path string, stat *unix.Stat_t) error {
	for {
		err := unix.Stat(path, stat)
		if err == nil {
			return nil
		}
		if e, ok := err.(temporaryer); ok && e.Temporary() {
			continue
		}
		return err
	}
}

// Mknod wrapped a golang.org/x/sys/unix.Mknod function to handle EINTR signal for Go 1.14+
func Mknod(path string, mode uint32, dev int) (err error) {
	for {
		err := unix.Mknod(path, mode, dev)
		if err == nil {
			return nil
		}
		if e, ok := err.(temporaryer); ok && e.Temporary() {
			continue
		}
		return err
	}
}

// Statfs wrapped a golang.org/x/sys/unix.Statfs function to handle EINTR signal for Go 1.14+
func Statfs(path string, buf *unix.Statfs_t) (err error) {
	for {
		err := unix.Statfs(path, buf)
		if err == nil {
			return nil
		}
		if e, ok := err.(temporaryer); ok && e.Temporary() {
			continue
		}
		return err
	}
}
