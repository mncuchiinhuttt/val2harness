//go:build !windows

package exporter

import "os"

func createNewPrivateFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
}

func createNewPrivateDirectory(root *os.Root, name string) error {
	return root.Mkdir(name, 0700)
}
