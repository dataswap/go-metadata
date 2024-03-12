package utils

import (
	"os"
	"syscall"
)

// FileLock represents a file lock.
type FileLock struct {
	lockFile *os.File
}

// NewFileLock creates a new FileLock instance for the specified file path.
func NewFileLock(filePath string) (*FileLock, error) {
	lockFilePath := filePath + ".lock"
	lockFile, err := os.OpenFile(lockFilePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	return &FileLock{lockFile: lockFile}, nil
}

// Lock acquires an exclusive lock on the file.
func (f *FileLock) Lock() error {
	return syscall.Flock(int(f.lockFile.Fd()), syscall.LOCK_EX)
}

// Unlock releases the lock on the file.
func (f *FileLock) Unlock() error {
	return syscall.Flock(int(f.lockFile.Fd()), syscall.LOCK_UN)
}

// Close closes the lock file.
func (f *FileLock) Close() {
	f.lockFile.Close()
}
