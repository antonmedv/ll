package main

import (
	"bytes"
	"fmt"
	"golang.org/x/crypto/ssh/terminal"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	. "strings"
)

const (
	modified  = "\033[1;34m%s\033[0m"
	added     = "\033[0;32m%s\033[0m"
	untracked = "\033[0;36m%s\033[0m"
)

func main() {
	// We are going to list files in current directory, if no arguments.
	if len(os.Args) > 1 {
		for i := 1; i < len(os.Args); i++ {
			if len(os.Args) > 2 {
				fmt.Printf("%v:\n", os.Args[i])
				ll(os.Args[i])
				fmt.Println()
			} else {
				ll(os.Args[i])
			}
		}
		return
	}

	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	ll(pwd)
}

func ll(cwd string) {
	// ReadDir already returns files and dirs sorted by filename.
	files, err := ioutil.ReadDir(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(files) == 0 {
		return
	}

	// We need terminal size to nicely fit on screen.
	fd := int(os.Stdin.Fd())
	width, height, err := terminal.GetSize(fd)
	if err != nil {
		width, height = 80, 60
	}

	// If it's possible to fit all files in one column on half of screen, just use one column.
	// Otherwise let's squeeze listing in half of screen.
	columns := len(files)/(height/2) + 1

	// Gonna keep file names and format string for git status.
	modes := map[string]string{}

	// If stdout of ll piped, use ls behavior: one line, no colors.
	fi, err := os.Stdout.Stat()
	if err != nil {
		panic(err)
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		columns = 1
	} else {
		status := gitStatus()
		for _, file := range files {
			name := file.Name()
			if file.IsDir() {
				name += "/"
			}
			// gitStatus returns file names of modified files from repo root.
			fullPath := filepath.Join(cwd, name)
			if file.IsDir() {
				fullPath += "/"
			}
			for path, mode := range status {
				// Use HasPrefix instead of exact mach to highlight directories as well.
				if HasPrefix(path, fullPath) {
					if mode[0] == '?' || mode[1] == '?' {
						modes[name] = untracked
					} else if mode[0] == 'A' || mode[1] == 'A' {
						modes[name] = added
					} else if mode[0] == 'M' || mode[1] == 'M' {
						modes[name] = modified
					}
				}
			}
		}
	}

start:
	// Let's try to fit everything in terminal width with this name columns.
	// If we are not able to do it, decrease column number and goto start.
	rows := int(math.Ceil(float64(len(files)) / float64(columns)))
	names := make([][]string, columns)
	n := 0
	for i := 0; i < columns; i++ {
		names[i] = make([]string, rows)
		// Columns size is going to be of max file name size.
		max := 0
		for j := 0; j < rows; j++ {
			name := ""
			if n < len(files) {
				name = files[n].Name()
				if files[n].IsDir() {
					// Dir should have slash at end.
					name += "/"
				}
				n++
			}
			if max < len(name) {
				max = len(name)
			}
			names[i][j] = name
		}
		// Append spaces to make all names in one column of same size.
		for j := 0; j < rows; j++ {
			names[i][j] += Repeat(" ", max-len(names[i][j]))
		}
	}

	const separator = "    " // Separator between columns.
	for j := 0; j < rows; j++ {
		row := make([]string, columns)
		for i := 0; i < columns; i++ {
			row[i] = names[i][j]
		}
		if len(Join(row, separator)) > width && columns > 1 {
			// Yep. No luck, let's decrease number of columns and try one more time.
			columns--
			goto start
		}
	}

	// Let's add colors from git status to file names.
	output := make([]string, rows)
	for j := 0; j < rows; j++ {
		row := make([]string, columns)
		for i := 0; i < columns; i++ {
			f, ok := modes[TrimRight(names[i][j], " ")]
			if !ok {
				f = "%s"
			}
			row[i] = fmt.Sprintf(f, names[i][j])
		}
		output[j] = Join(row, separator)
	}
	fmt.Println(Join(output, "\n"))
}

func gitRepo() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return Trim(out.String(), "\n"), err
}

func gitStatus() map[string]string {
	repo, err := gitRepo()
	if err != nil {
		return nil
	}
	cmd := exec.Command("git", "status", "--porcelain=v1")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		return nil
	}
	m := map[string]string{}
	for _, line := range Split(Trim(out.String(), "\n"), "\n") {
		if len(line) == 0 {
			continue
		}
		m[filepath.Join(repo, line[3:])] = line[:2]
	}
	return m
}
