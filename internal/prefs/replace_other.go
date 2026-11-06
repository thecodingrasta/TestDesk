//go:build !windows

package prefs

import "os"

// POSIX rename and open already succeed while other processes have the file open.
func replaceFile(from, to string) error    { return os.Rename(from, to) }
func readFile(path string) ([]byte, error) { return os.ReadFile(path) }
