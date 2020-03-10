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
	"sync"
	"time"
)

const (
	modified  = "\033[1;34m%s\033[0m"
	added     = "\033[0;32m%s\033[0m"
	untracked = "\033[0;36m%s\033[0m"
	bold      = "\033[1m%v\033[0m"
)

var (
	spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	sizes   = []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}
	base    = float64(1000)
)

func main() {
	if len(os.Args) == 2 {
		ll(os.Args[1])
		return
	}

	if len(os.Args) > 2 {
		for i := 1; i < len(os.Args); i++ {
			path, _ := filepath.Abs(os.Args[i])
			printInfo(fileInfo(path), path)
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
	// Maybe it is and argument, so get absolute path.
	cwd, _ = filepath.Abs(cwd)

	// Is it a file?
	if fi := fileInfo(cwd); !fi.IsDir() {
		printInfo(fi, cwd)
		return
	}

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
			for path, mode := range status {
				if subPath(path, fullPath) {
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
	// Let's try to fit everything in terminal width with this many columns.
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

func subPath(path string, fullPath string) bool {
	p := Split(path, "/")
	for i, s := range Split(fullPath, "/") {
		if i >= len(p) {
			return false
		}
		if p[i] != s {
			return false
		}
	}
	return true
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

func printInfo(fi os.FileInfo, path string) {
	name := fi.Name()
	size := fi.Size()
	if fi.IsDir() {
		name += "/"
		done := make(chan bool)
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			i, t := 0, time.Tick(100*time.Millisecond)
			for {
				select {
				case <-t:
					fmt.Printf("\r%v\t%v", spinner[i%len(spinner)], name)
					i++
				case <-done:
					fmt.Print("\r")
					return
				}
			}
		}()
		size, _ = dirSize(path)
		done <- true
		wg.Wait()
	}
	fmt.Printf("%v\t%v\n", toHuman(size), name)
}

func fileInfo(path string) os.FileInfo {
	fi, err := os.Stat(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return fi
}

func toHuman(s int64) string {
	if s < 10 {
		value := fmt.Sprintf(bold, s)
		return fmt.Sprintf("  %v B", value)
	}
	e := math.Floor(math.Log(float64(s)) / math.Log(base))
	suffix := sizes[int(e)]
	val := math.Floor(float64(s)/math.Pow(base, e)*10+0.5) / 10
	f := "%3.0f"
	if val < 10 {
		f = "%3.1f"
	}

	value := fmt.Sprintf(bold, fmt.Sprintf(f, val))
	return fmt.Sprintf("%v %v", value, suffix)
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}
