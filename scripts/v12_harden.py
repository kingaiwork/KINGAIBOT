#!/usr/bin/env python3
from pathlib import Path
import re


def must_replace(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"patch target not found: {label}")
    return text.replace(old, new, 1)


# Production language/toolchain baseline.
p = Path("go.mod")
s = p.read_text()
s = re.sub(r"(?m)^go\s+[^\n]+$", "go 1.26.5", s, count=1)
p.write_text(s)

# Tool execution hardening.
p = Path("internal/tool/registry.go")
s = p.read_text()
start = s.index("func (r *Registry) fileRead(")
middle = s.index("func (r *Registry) fileWrite(", start)
end = s.index("func (r *Registry) httpGet(", middle)
file_read = '''func (r *Registry) fileRead(raw json.RawMessage) (string, error) {
\tvar a struct {
\t\tPath string `json:"path"`
\t}
\tif err := json.Unmarshal(raw, &a); err != nil {
\t\treturn "", err
\t}
\tf, err := storage.OpenAllowedFile(a.Path, r.cfg.Security.FileReadRoots)
\tif err != nil {
\t\treturn "", err
\t}
\tdefer f.Close()
\tb, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
\tif err != nil {
\t\treturn "", err
\t}
\tif len(b) > 2<<20 {
\t\treturn "", errors.New("file exceeds 2 MiB limit")
\t}
\treturn string(b), nil
}

'''
file_write = '''func (r *Registry) fileWrite(raw json.RawMessage) (string, error) {
\tvar a struct {
\t\tPath    string `json:"path"`
\t\tContent string `json:"content"`
\t}
\tif err := json.Unmarshal(raw, &a); err != nil {
\t\treturn "", err
\t}
\tif len(a.Content) > 2<<20 {
\t\treturn "", errors.New("content exceeds 2 MiB limit")
\t}
\tif err := storage.AtomicWriteAllowedFile(a.Path, r.cfg.Security.FileWriteRoots, []byte(a.Content), 0o600); err != nil {
\t\treturn "", err
\t}
\treturn `{"ok":true}`, nil
}

'''
s = s[:start] + file_read + file_write + s[end:]

old = '''\tif u.Hostname() == "" {
\t\treturn "", errors.New("URL hostname required")
\t}
\tif !hostAllowed(u.Hostname(), r.cfg.Security.HTTPAllowedHosts) {'''
new = '''\tif u.Hostname() == "" {
\t\treturn "", errors.New("URL hostname required")
\t}
\tif port := u.Port(); port != "" && port != "443" {
\t\treturn "", errors.New("http_get permits only the standard HTTPS port")
\t}
\tif !hostAllowed(u.Hostname(), r.cfg.Security.HTTPAllowedHosts) {'''
s = must_replace(s, old, new, "http initial port validation")

old = '''\tclient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
\t\tif len(via) >= 5 {
\t\t\treturn errors.New("too many redirects")
\t\t}
\t\tif req.URL.User != nil {
\t\t\treturn errors.New("redirect URL credentials denied")
\t\t}
\t\tif !hostAllowed(req.URL.Hostname(), r.cfg.Security.HTTPAllowedHosts) {
\t\t\treturn errors.New("redirect host denied")
\t\t}
\t\treturn nil
\t}'''
new = '''\tclient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
\t\tif len(via) >= 5 {
\t\t\treturn errors.New("too many redirects")
\t\t}
\t\tif req.URL.User != nil {
\t\t\treturn errors.New("redirect URL credentials denied")
\t\t}
\t\tif req.URL.Scheme != "https" {
\t\t\treturn errors.New("redirect protocol downgrade denied")
\t\t}
\t\tif port := req.URL.Port(); port != "" && port != "443" {
\t\t\treturn errors.New("redirect non-standard HTTPS port denied")
\t\t}
\t\tif !hostAllowed(req.URL.Hostname(), r.cfg.Security.HTTPAllowedHosts) {
\t\t\treturn errors.New("redirect host denied")
\t\t}
\t\treturn nil
\t}'''
s = must_replace(s, old, new, "http redirect validation")

old = '''\tclient := netguard.Client(60*time.Second, ep.AllowPrivateNetwork)
\tresp, err := client.Do(req)'''
new = '''\tclient := netguard.Client(60*time.Second, ep.AllowPrivateNetwork)
\tclient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
\tresp, err := client.Do(req)'''
s = must_replace(s, old, new, "remote RPC redirects")
s = s.replace("KING-Agent-OS/1.1", "KINGAIBOT/1.2")
p.write_text(s)

# Model provider credentials must not be forwarded through redirects.
p = Path("internal/provider/openai.go")
s = p.read_text()
old = '''\tclient := netguard.Client(c.timeout, p.AllowPrivateNetwork)
\tresp, err := client.Do(req)'''
new = '''\tclient := netguard.Client(c.timeout, p.AllowPrivateNetwork)
\tclient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
\tresp, err := client.Do(req)'''
s = must_replace(s, old, new, "provider redirects")
s = s.replace("KING-Agent-OS/1.1", "KINGAIBOT/1.2")
p.write_text(s)

# Validate network and command capability configuration at startup.
p = Path("internal/config/config.go")
s = p.read_text()
needle = '''\tfor i := range c.Security.FileWriteRoots {
\t\tc.Security.FileWriteRoots[i] = abs(base, c.Security.FileWriteRoots[i])
\t}
'''
insert = needle + '''\tfor _, host := range c.Security.HTTPAllowedHosts {
\t\thost = strings.TrimSpace(host)
\t\tif host == "*" {
\t\t\treturn errors.New("security.http_allowed_hosts does not permit a global wildcard")
\t\t}
\t\tif host == "" || strings.ContainsAny(host, "/@?#") {
\t\t\treturn fmt.Errorf("invalid HTTP allowed host %q", host)
\t\t}
\t}
\tfor _, command := range c.Security.ShellAllowlist {
\t\tif command == "" || command != filepath.Base(command) || strings.ContainsAny(command, "/\\\\") || strings.IndexByte(command, 0) >= 0 || len(command) > 128 {
\t\t\treturn fmt.Errorf("invalid shell allowlist command %q; only bare command names are allowed", command)
\t\t}
\t}
'''
s = must_replace(s, needle, insert, "config security validation")
p.write_text(s)

# Versioned example configuration.
p = Path("configs/config.example.json")
s = p.read_text().replace('"version": "1.1.0"', '"version": "1.2.0"', 1)
p.write_text(s)

# Execution-path regression test for the os.Root-backed tool calls.
p = Path("internal/tool/registry_test.go")
s = p.read_text()
if "TestFileToolRejectsSymlinkEscapeWithOSRoot" not in s:
    s += '''

func TestFileToolRejectsSymlinkEscapeWithOSRoot(t *testing.T) {
\tif runtime.GOOS == "windows" {
\t\tt.Skip("symlink creation permissions vary on Windows")
\t}
\troot := t.TempDir()
\toutside := t.TempDir()
\tsecret := filepath.Join(outside, "secret.txt")
\tif err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
\t\tt.Fatal(err)
\t}
\tlink := filepath.Join(root, "escape")
\tif err := os.Symlink(secret, link); err != nil {
\t\tt.Fatal(err)
\t}
\tcfg := &config.Config{Security: config.Security{FileReadRoots: []string{root}, FileWriteRoots: []string{root}}}
\treg := New(cfg, nil, nil, nil)
\treadArgs, _ := json.Marshal(map[string]any{"path": link})
\tif _, err := reg.fileRead(readArgs); err == nil {
\t\tt.Fatal("expected read through escaping symlink to be rejected")
\t}
\twriteArgs, _ := json.Marshal(map[string]any{"path": filepath.Join(root, "escape", "pwned.txt"), "content": "blocked"})
\tif _, err := reg.fileWrite(writeArgs); err == nil {
\t\tt.Fatal("expected write through escaping symlink to be rejected")
\t}
\tif _, err := os.Stat(filepath.Join(outside, "pwned.txt")); !os.IsNotExist(err) {
\t\tt.Fatalf("outside file unexpectedly created: %v", err)
\t}
}
'''
p.write_text(s)
