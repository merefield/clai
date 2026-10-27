// Package guidance discovers bounded, provenance-labelled local instructions.
package guidance

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type Locations struct {
	CWD, Home, Config, Project string
	Chain                      []string // project root to CWD, inclusive
}

func Locate() (Locations, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Locations{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Locations{}, err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return Locations{}, err
	}
	return FindLocations(cwd, home, config)
}

func FindLocations(cwd, home, config string) (Locations, error) {
	return findLocations(cwd, home, config, func(directory string) bool {
		info, err := os.Stat(filepath.Join(directory, ".git"))
		return err == nil && (info.IsDir() || info.Mode().IsRegular())
	})
}

func findLocations(cwd, home, config string, hasGit func(string) bool) (Locations, error) {
	cwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return Locations{}, err
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return Locations{}, err
	}
	root := cwd
	for directory := cwd; ; directory = filepath.Dir(directory) {
		if hasGit(directory) {
			root = directory
			break
		}
		if filepath.Dir(directory) == directory {
			break
		}
	}
	chain := []string{}
	for directory := cwd; ; directory = filepath.Dir(directory) {
		chain = append(chain, directory)
		if directory == root {
			break
		}
	}
	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}
	return Locations{CWD: cwd, Home: home, Config: filepath.Join(config, "clai"), Project: root, Chain: chain}, nil
}

func Display(path string) string { return filepath.ToSlash(filepath.Clean(path)) }

// Open confines resolution to root, including symlinks and races during open.
// Absolute symlinks are deliberately unsupported by os.Root, even when their
// target appears to be inside root. Relative symlinks may stay inside root.
func Open(root, path string) (*os.File, string, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(relative) {
		return nil, "", fmt.Errorf("path is outside the permitted root")
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		return nil, "", err
	}
	defer directory.Close()
	info, err := directory.Stat(relative)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("expected a regular file")
	}
	file, err := directory.Open(relative)
	if err != nil {
		return nil, "", err
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, "", fmt.Errorf("expected a regular file")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		file.Close()
		return nil, "", err
	}
	return file, canonical, nil
}

func Read(root, path string, maximum int) (string, string, int, error) {
	file, canonical, err := Open(root, path)
	if err != nil {
		return "", "", 0, err
	}
	defer file.Close()
	text, size, err := readText(file, maximum)
	return text, canonical, size, err
}

func readText(reader io.Reader, maximum int) (string, int, error) {
	data, err := io.ReadAll(io.LimitReader(reader, int64(maximum)+1))
	if err != nil {
		return "", 0, err
	}
	if len(data) > maximum {
		return "", 0, fmt.Errorf("exceeds the %d-byte limit; omitted", maximum)
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
		return "", 0, fmt.Errorf("expected UTF-8 text without NUL bytes")
	}
	return strings.TrimPrefix(string(data), "\ufeff"), len(data), nil
}

func OpenDirectory(boundary, path string) (*os.Root, error) {
	relative, err := filepath.Rel(boundary, path)
	if err != nil || !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("directory is outside the permitted root")
	}
	root, err := os.OpenRoot(boundary)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.OpenRoot(relative)
}

// ReadAt uses a pinned directory handle for a logical request. Replacing the
// skill directory with a symlink after activation cannot redirect this read.
func ReadAt(root *os.Root, path string, maximum int) (string, int, error) {
	if root == nil || !filepath.IsLocal(path) {
		return "", 0, fmt.Errorf("unavailable or invalid documentation root")
	}
	info, err := root.Stat(path)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("expected a regular file")
	}
	file, err := root.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("expected a regular file")
	}
	return readText(file, maximum)
}

func EntriesAt(root *os.Root, path string, maximum int) ([]os.DirEntry, error) {
	file, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(maximum + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > maximum {
		return nil, fmt.Errorf("more than %d directory entries; omitted", maximum)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// EntriesWithin never allocates an unbounded directory listing. Oversized directories
// are omitted entirely rather than choosing a filesystem-dependent subset.
func EntriesWithin(root, path string, maximum int) ([]os.DirEntry, error) {
	directory, err := OpenDirectory(root, path)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	return EntriesAt(directory, ".", maximum)
}
