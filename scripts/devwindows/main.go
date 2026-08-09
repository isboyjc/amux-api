//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
)

type childProcess struct {
	name string
	cmd  *exec.Cmd
}

type processResult struct {
	name string
	err  error
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[dev] %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}

	goPath, err := findGo()
	if err != nil {
		return err
	}
	bunPath, err := findBun()
	if err != nil {
		return err
	}

	webDir := filepath.Join(repoRoot, "web")
	if _, err := os.Stat(filepath.Join(webDir, "node_modules")); errors.Is(err, os.ErrNotExist) {
		fmt.Println("[dev] Frontend dependencies are missing; running bun install...")
		if err := runBun(webDir, bunPath, "install"); err != nil {
			return fmt.Errorf("bun install failed: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("check frontend dependencies: %w", err)
	}

	frontend := &childProcess{
		name: "frontend",
		cmd:  bunCommand(webDir, bunPath, "run", "dev"),
	}

	backend, hotReload := backendCommand(repoRoot, goPath)
	if hotReload {
		fmt.Println("[dev] Backend hot reload: Air")
	} else {
		fmt.Println("[dev] Air was not found; backend will run without hot reload.")
		fmt.Println("[dev] Optional install: go install github.com/air-verse/air@latest")
	}

	children := []*childProcess{backend, frontend}
	for _, child := range children {
		attachConsole(child.cmd)
		if err := child.cmd.Start(); err != nil {
			stopChildren(children)
			return fmt.Errorf("start %s: %w", child.name, err)
		}
		fmt.Printf("[dev] Started %s (PID %d)\n", child.name, child.cmd.Process.Pid)
	}

	results := make(chan processResult, len(children))
	for _, child := range children {
		go func(child *childProcess) {
			results <- processResult{name: child.name, err: child.cmd.Wait()}
		}(child)
	}

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)

	var result processResult
	select {
	case <-interrupts:
		fmt.Println("\n[dev] Stopping frontend and backend...")
		stopChildren(children)
		return nil
	case result = <-results:
		fmt.Printf("[dev] %s stopped; shutting down the other process.\n", result.name)
		stopChildren(children)
	}

	if result.err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(result.err, &exitErr) {
		return fmt.Errorf("%s exited with code %d", result.name, exitErr.ExitCode())
	}
	return fmt.Errorf("wait for %s: %w", result.name, result.err)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "web", "package.json")); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root was not found")
		}
		dir = parent
	}
}

func findGo() (string, error) {
	if path, err := exec.LookPath("go.exe"); err == nil {
		return path, nil
	}
	if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
		path := filepath.Join(programFiles, "Go", "bin", "go.exe")
		if fileExists(path) {
			return path, nil
		}
	}
	return "", errors.New("Go was not found; install 64-bit Go and reopen the terminal")
}

func findBun() (string, error) {
	for _, name := range []string{"bun.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		path := filepath.Join(appData, "npm", `node_modules`, `bun`, `bin`, `bun.exe`)
		if fileExists(path) {
			return path, nil
		}
	}
	if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
		path := filepath.Join(userProfile, ".bun", "bin", "bun.exe")
		if fileExists(path) {
			return path, nil
		}
	}
	return "", errors.New("Bun was not found; install Bun and reopen the terminal")
}

func backendCommand(repoRoot, goPath string) (*childProcess, bool) {
	if airPath := findAir(goPath); airPath != "" {
		return &childProcess{
			name: "backend",
			cmd: commandInDir(
				repoRoot,
				airPath,
				"-c",
				filepath.Join(repoRoot, ".air.windows.toml"),
			),
		}, true
	}
	return &childProcess{
		name: "backend",
		cmd:  commandInDir(repoRoot, goPath, "run", "."),
	}, false
}

func findAir(goPath string) string {
	if path, err := exec.LookPath("air.exe"); err == nil {
		return path
	}
	cmd := exec.Command(goPath, "env", "GOPATH")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, goPathDir := range filepath.SplitList(strings.TrimSpace(string(output))) {
		candidate := filepath.Join(goPathDir, "bin", "air.exe")
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func runBun(dir, bunPath string, args ...string) error {
	cmd := bunCommand(dir, bunPath, args...)
	attachConsole(cmd)
	return cmd.Run()
}

func bunCommand(dir, bunPath string, args ...string) *exec.Cmd {
	if strings.EqualFold(filepath.Ext(bunPath), ".cmd") {
		comspec := os.Getenv("ComSpec")
		if comspec == "" {
			comspec = filepath.Join(os.Getenv("WINDIR"), "System32", "cmd.exe")
		}
		commandLine := `call ` + quoteCmdArg(bunPath)
		for _, arg := range args {
			commandLine += " " + quoteCmdArg(arg)
		}
		return commandInDir(dir, comspec, "/d", "/s", "/c", commandLine)
	}
	return commandInDir(dir, bunPath, args...)
}

func commandInDir(dir, path string, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.Dir = dir
	return cmd
}

func attachConsole(cmd *exec.Cmd) {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

func quoteCmdArg(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func stopChildren(children []*childProcess) {
	var wg sync.WaitGroup
	for _, child := range children {
		if child == nil || child.cmd == nil || child.cmd.Process == nil {
			continue
		}
		wg.Add(1)
		go func(child *childProcess) {
			defer wg.Done()
			stopProcessTree(child.cmd.Process.Pid)
		}(child)
	}
	wg.Wait()
}

func stopProcessTree(pid int) {
	taskkill := filepath.Join(os.Getenv("WINDIR"), "System32", "taskkill.exe")
	cmd := exec.Command(taskkill, "/PID", fmt.Sprintf("%d", pid), "/T", "/F")
	_ = cmd.Run()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
