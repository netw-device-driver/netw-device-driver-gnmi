package util

import "os"

// CreateDirectory creates a directory by a path with a mode/permission specified by perm.
// If directory exists, the function does not do anything.
func CreateDirectory(path string, perm os.FileMode) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.Mkdir(path, perm)
	}
}

// CreateFile function
func CreateFile(file, content string) {
	var f *os.File
	f, err := os.Create(file)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if _, err := f.WriteString(content + "\n"); err != nil {
		panic(err)
	}
}
