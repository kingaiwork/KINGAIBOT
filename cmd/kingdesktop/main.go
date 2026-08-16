package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var version = "1.5.0-dev"

func main() {
	consoleURL := flag.String("url", "http://127.0.0.1:18889/ui/", "KING AI Control Center URL")
	consoleBin := flag.String("console", "", "path to kingconsole; defaults to sibling binary or PATH")
	noOpen := flag.Bool("no-open", false, "start/check the visual client without opening a browser")
	wait := flag.Duration("wait", 12*time.Second, "maximum time to wait for the local visual client")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	if !isLocalConsoleURL(*consoleURL) {
		fmt.Fprintln(os.Stderr, "KING AI Desktop only opens a loopback Control Center URL")
		os.Exit(2)
	}

	if !ready(*consoleURL) {
		bin, err := resolveConsole(*consoleBin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "visual client:", err)
			os.Exit(1)
		}
		if err := startConsole(bin); err != nil {
			fmt.Fprintln(os.Stderr, "visual client: cannot start kingconsole:", err)
			os.Exit(1)
		}
	}

	deadline := time.Now().Add(*wait)
	for !ready(*consoleURL) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if !ready(*consoleURL) {
		fmt.Fprintln(os.Stderr, "visual client did not become ready:", *consoleURL)
		os.Exit(1)
	}

	fmt.Println("KING AI Control Center:", *consoleURL)
	if *noOpen {
		return
	}
	if err := openBrowser(*consoleURL); err != nil {
		fmt.Fprintln(os.Stderr, "open browser:", err)
		fmt.Println("Open this address manually:", *consoleURL)
		os.Exit(1)
	}
}

func isLocalConsoleURL(raw string) bool {
	return strings.HasPrefix(raw, "http://127.0.0.1:") || strings.HasPrefix(raw, "http://localhost:") || strings.HasPrefix(raw, "http://[::1]:")
}

func ready(raw string) bool {
	client := &http.Client{Timeout: 1200 * time.Millisecond, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(raw)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}

func resolveConsole(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}
	exe, err := os.Executable()
	if err == nil {
		name := "kingconsole"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}
	name := "kingconsole"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", errors.New("kingconsole is not installed beside kingdesktop or in PATH")
}

func startConsole(bin string) error {
	cmd := exec.Command(bin, "-listen", "127.0.0.1:18889", "-api", "http://127.0.0.1:18888")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func openBrowser(raw string) error {
	var candidates [][]string
	switch runtime.GOOS {
	case "windows":
		candidates = [][]string{{"rundll32", "url.dll,FileProtocolHandler", raw}}
	case "darwin":
		candidates = [][]string{{"open", raw}}
	default:
		candidates = [][]string{{"xdg-open", raw}, {"gio", "open", raw}}
	}
	var last error
	for _, parts := range candidates {
		if _, err := exec.LookPath(parts[0]); err != nil {
			last = err
			continue
		}
		if err := exec.Command(parts[0], parts[1:]...).Start(); err == nil {
			return nil
		} else {
			last = err
		}
	}
	if last == nil {
		last = errors.New("no supported browser opener found")
	}
	return last
}
