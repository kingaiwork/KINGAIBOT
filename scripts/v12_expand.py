#!/usr/bin/env python3
from pathlib import Path


def replace_once(s: str, old: str, new: str, label: str) -> str:
    if old not in s:
        raise SystemExit(f"patch target not found: {label}")
    return s.replace(old, new, 1)

p = Path("internal/tool/registry.go")
s = p.read_text()

anchor = '''\t\tdef("file_write", "Write a UTF-8 text file inside approved roots", map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, "required": []string{"path", "content"}}),
'''
extra = anchor + '''\t\tdef("file_stat", "Read metadata for a file or directory inside approved roots", map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}),
\t\tdef("file_list", "List one directory inside approved roots", map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}),
\t\tdef("file_mkdir", "Create a directory tree inside approved write roots", map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}),
\t\tdef("file_delete", "Delete one file or one empty directory inside approved write roots", map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}),
'''
s = replace_once(s, anchor, extra, "tool definitions")

anchor = '''\tcase "file_write":
\t\treturn r.fileWrite(args)
'''
extra = anchor + '''\tcase "file_stat":
\t\treturn r.fileStat(args)
\tcase "file_list":
\t\treturn r.fileList(args)
\tcase "file_mkdir":
\t\treturn r.fileMkdir(args)
\tcase "file_delete":
\t\treturn r.fileDelete(args)
'''
s = replace_once(s, anchor, extra, "tool dispatch")

insert_at = s.index("func (r *Registry) httpGet(")
methods = '''func (r *Registry) fileStat(raw json.RawMessage) (string, error) {
\tvar a struct { Path string `json:"path"` }
\tif err := json.Unmarshal(raw, &a); err != nil { return "", err }
\tinfo, err := storage.StatAllowedPath(a.Path, r.cfg.Security.FileReadRoots)
\tif err != nil { return "", err }
\tb, err := json.Marshal(info)
\tif err != nil { return "", err }
\treturn string(b), nil
}

func (r *Registry) fileList(raw json.RawMessage) (string, error) {
\tvar a struct { Path string `json:"path"` }
\tif err := json.Unmarshal(raw, &a); err != nil { return "", err }
\tentries, err := storage.ListAllowedDir(a.Path, r.cfg.Security.FileReadRoots)
\tif err != nil { return "", err }
\tb, err := json.Marshal(entries)
\tif err != nil { return "", err }
\treturn string(b), nil
}

func (r *Registry) fileMkdir(raw json.RawMessage) (string, error) {
\tvar a struct { Path string `json:"path"` }
\tif err := json.Unmarshal(raw, &a); err != nil { return "", err }
\tif err := storage.MkdirAllowed(a.Path, r.cfg.Security.FileWriteRoots); err != nil { return "", err }
\treturn `{"ok":true}`, nil
}

func (r *Registry) fileDelete(raw json.RawMessage) (string, error) {
\tvar a struct { Path string `json:"path"` }
\tif err := json.Unmarshal(raw, &a); err != nil { return "", err }
\tif err := storage.RemoveAllowed(a.Path, r.cfg.Security.FileWriteRoots); err != nil { return "", err }
\treturn `{"ok":true}`, nil
}

'''
s = s[:insert_at] + methods + s[insert_at:]
p.write_text(s)

p = Path("configs/config.example.json")
s = p.read_text()
anchor = '''      "file_write": "ask",
'''
extra = anchor + '''      "file_stat": "allow",
      "file_list": "allow",
      "file_mkdir": "ask",
      "file_delete": "ask",
'''
s = replace_once(s, anchor, extra, "example policies")
p.write_text(s)

# Product archive branding. Workflow YAML is intentionally not modified from
# the low-privilege Actions token; release.yml is updated separately through
# the repository control plane after this validated migration lands.
for name in [
    "scripts/build-release.sh", "scripts/install.sh", "scripts/update.sh",
    "scripts/install.ps1", "scripts/update.ps1",
]:
    p = Path(name)
    s = p.read_text().replace("king-agent-os_", "kingaibot_")
    if name == "scripts/build-release.sh":
        s = s.replace("${GITHUB_REF_NAME:-1.1.0}", "${GITHUB_REF_NAME:-1.2.0}")
    p.write_text(s)

p = Path("internal/tool/registry_test.go")
s = p.read_text()
if "TestSafeFileManagementTools" not in s:
    s += '''

func TestSafeFileManagementTools(t *testing.T) {
\troot := t.TempDir()
\tcfg := &config.Config{Security: config.Security{FileReadRoots: []string{root}, FileWriteRoots: []string{root}}}
\treg := New(cfg, nil, nil, nil)
\tdir := filepath.Join(root, "nested")
\tmkdirArgs, _ := json.Marshal(map[string]any{"path": dir})
\tif _, err := reg.fileMkdir(mkdirArgs); err != nil { t.Fatal(err) }
\tfile := filepath.Join(dir, "a.txt")
\tif err := os.WriteFile(file, []byte("x"), 0o600); err != nil { t.Fatal(err) }
\tstatArgs, _ := json.Marshal(map[string]any{"path": file})
\tif out, err := reg.fileStat(statArgs); err != nil || !strings.Contains(out, `"name":"a.txt"`) { t.Fatalf("stat: %v %s", err, out) }
\tlistArgs, _ := json.Marshal(map[string]any{"path": dir})
\tif out, err := reg.fileList(listArgs); err != nil || !strings.Contains(out, `"name":"a.txt"`) { t.Fatalf("list: %v %s", err, out) }
\tdeleteArgs, _ := json.Marshal(map[string]any{"path": file})
\tif _, err := reg.fileDelete(deleteArgs); err != nil { t.Fatal(err) }
\tif _, err := os.Stat(file); !os.IsNotExist(err) { t.Fatalf("file still exists: %v", err) }
}
'''
p.write_text(s)
