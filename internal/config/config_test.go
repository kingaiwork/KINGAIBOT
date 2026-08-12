package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string { t.Helper(); p := filepath.Join(t.TempDir(), "config.json"); if err := os.WriteFile(p, []byte(body), 0o600); err != nil { t.Fatal(err) }; return p }
func TestLoadAllowsExplicitMemoryDisable(t *testing.T) {
	p := writeConfig(t, `{"server":{"admin_token_env":"ADMIN","mcp_token_env":"MCP","a2a_token_env":"A2A"},"runtime":{"data_dir":"./data","workspace_dir":"./data/workspace"},"memory":{"enabled":false,"store_task_inputs":false,"store_task_outputs":false},"providers":[{"name":"p","base_url":"https://example.com/v1","model":"m","enabled":true}],"security":{"default_tool_policy":"deny"},"evolution":{"mode":"proposal-only"},"protocols":{"mcp":true,"a2a":true}}`)
	c, err := Load(p); if err != nil { t.Fatal(err) }; if c.Memory.Enabled || c.Memory.StoreTaskInputs || c.Memory.StoreTaskOutputs { t.Fatalf("explicit memory disable was overwritten: %#v", c.Memory) }
}
func TestLoadLegacyConfigGetsSafeMemoryDefault(t *testing.T) {
	p := writeConfig(t, `{"server":{"admin_token_env":"ADMIN","mcp_token_env":"MCP","a2a_token_env":"A2A"},"providers":[{"name":"p","base_url":"https://example.com/v1","model":"m","enabled":true}],"security":{"default_tool_policy":"deny"},"evolution":{"mode":"proposal-only"},"protocols":{"mcp":true,"a2a":true}}`)
	c, err := Load(p); if err != nil { t.Fatal(err) }; if !c.Memory.Enabled || c.Memory.StoreTaskInputs || !c.Memory.StoreTaskOutputs { t.Fatalf("unexpected legacy memory default: %#v", c.Memory) }
}
func TestProtocolTokensMustBeSeparated(t *testing.T) { c := &Config{Server: Server{AdminTokenEnv:"SAME",MCPTokenEnv:"SAME",A2ATokenEnv:"A2A"},Providers:[]Provider{{Name:"p",BaseURL:"https://example.com",Model:"m",Enabled:true}},Security:Security{DefaultToolPolicy:"deny"},Evolution:Evolution{Mode:"proposal-only"},Protocols:Protocols{MCP:true,A2A:true}}; if err:=c.Normalize(t.TempDir());err==nil||!strings.Contains(err.Error(),"distinct"){t.Fatalf("expected token separation error, got %v",err)} }
func TestPublicBaseURLRequiresHTTPS(t *testing.T) { c := &Config{Server:Server{BaseURL:"http://example.com",AdminTokenEnv:"ADMIN",MCPTokenEnv:"MCP",A2ATokenEnv:"A2A"},Providers:[]Provider{{Name:"p",BaseURL:"https://example.com",Model:"m",Enabled:true}},Security:Security{DefaultToolPolicy:"deny"},Evolution:Evolution{Mode:"proposal-only"}}; if err:=c.Normalize(t.TempDir());err==nil{t.Fatal("expected public http base URL to be rejected")} }
