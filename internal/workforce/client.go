package workforce

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const defaultControlPlane = "https://api.kingai.work"
var serviceTokenPattern = regexp.MustCompile(`^ksv_[0-9a-fA-F]{64}$`)

type Settings struct {
	Enabled bool
	ControlPlaneURL string
	ServiceToken string
	AllowInsecureHTTP bool
	HeartbeatInterval time.Duration
	SyncInterval time.Duration
	PollInterval time.Duration
	RequestTimeout time.Duration
	ReportOutput bool
	MaxReportBytes int
	BindingsFile string
}

type EmployeeConnector struct { ID string `json:"id"`; ProviderKey string `json:"provider_key"`; Name string `json:"name"`; AuthMode string `json:"auth_mode"`; LocalAlias string `json:"local_alias"`; Skills []string `json:"skills"`; Config map[string]any `json:"config"` }
type Employee struct { ID string `json:"id"`; Name string `json:"name"`; Title string `json:"title"`; RoleKey string `json:"role_key"`; Status string `json:"status"`; AutonomyLevel string `json:"autonomy_level"`; RiskCeiling string `json:"risk_ceiling"`; Skills []string `json:"skills"`; Goals []string `json:"goals"`; Connectors []EmployeeConnector `json:"connectors,omitempty"` }
type Workflow struct { ID string `json:"id"`; EmployeeID string `json:"employee_id"`; Name string `json:"name"`; TriggerType string `json:"trigger_type"`; Status string `json:"status"`; RiskLevel string `json:"risk_level"`; Definition map[string]any `json:"definition"` }
type Connector struct { ID string `json:"id"`; ProviderKey string `json:"provider_key"`; Name string `json:"name"`; Status string `json:"status"`; AuthMode string `json:"auth_mode"`; LocalAlias string `json:"local_alias"`; AllowedSkills []string `json:"allowed_skills"`; Config map[string]any `json:"config"` }
type ConnectorBinding struct { EmployeeID string `json:"employee_id"`; ConnectorID string `json:"connector_id"`; Status string `json:"status"`; SkillScope []string `json:"skill_scope"` }
type CloudTask struct { ID string `json:"id"`; OrganizationID string `json:"organization_id"`; WorkspaceID string `json:"workspace_id"`; EmployeeID string `json:"employee_id"`; Title string `json:"title"`; Instructions string `json:"instructions"`; Priority string `json:"priority"`; RiskLevel string `json:"risk_level"`; ActionFingerprint string `json:"action_fingerprint"`; DueAt string `json:"due_at"`; CreatedAt string `json:"created_at"`; StartedAt string `json:"started_at"` }

type SyncResponse struct {
	OK bool `json:"ok"`
	Schema string `json:"schema"`
	SkillsSchema string `json:"skills_schema"`
	Employees []Employee `json:"employees"`
	Workflows []Workflow `json:"workflows"`
	Connectors []Connector `json:"connectors"`
	ConnectorBindings []ConnectorBinding `json:"connector_bindings"`
	Policy struct { CloudNeverBypassesLocalApproval bool `json:"cloud_never_bypasses_local_approval"`; ArbitraryShell bool `json:"arbitrary_shell"`; CredentialsInCloud bool `json:"credentials_in_cloud"`; ConnectorConfigGrantsPermission bool `json:"connector_config_grants_permission"`; GenericRemoteShell bool `json:"generic_remote_shell"`; ExecutionBoundary string `json:"execution_boundary"` } `json:"policy"`
}

type Client struct { base *url.URL; token string; version string; http *http.Client }

func SettingsFromEnv(dataDir string) (Settings,error) {
	token:=strings.TrimSpace(os.Getenv("KINGAI_WORKFORCE_SERVICE_TOKEN")); if token=="" { return Settings{Enabled:false},nil }
	if !serviceTokenPattern.MatchString(token) { return Settings{},errors.New("KINGAI_WORKFORCE_SERVICE_TOKEN has invalid format") }
	base:=strings.TrimSpace(os.Getenv("KINGAI_WORKFORCE_URL")); if base=="" { base=defaultControlPlane }
	bindings:=strings.TrimSpace(os.Getenv("KINGAI_WORKFORCE_BINDINGS_FILE")); if bindings=="" { bindings=filepathJoin(dataDir,"workforce","bindings.json") }
	s:=Settings{Enabled:true,ControlPlaneURL:base,ServiceToken:token,AllowInsecureHTTP:envBool("KINGAI_WORKFORCE_ALLOW_INSECURE_HTTP"),HeartbeatInterval:envDuration("KINGAI_WORKFORCE_HEARTBEAT_SECONDS",60*time.Second,15*time.Second,30*time.Minute),SyncInterval:envDuration("KINGAI_WORKFORCE_SYNC_SECONDS",120*time.Second,30*time.Second,time.Hour),PollInterval:envDuration("KINGAI_WORKFORCE_POLL_SECONDS",8*time.Second,2*time.Second,5*time.Minute),RequestTimeout:envDuration("KINGAI_WORKFORCE_REQUEST_TIMEOUT_SECONDS",30*time.Second,5*time.Second,2*time.Minute),ReportOutput:envBool("KINGAI_WORKFORCE_REPORT_OUTPUT"),MaxReportBytes:envInt("KINGAI_WORKFORCE_MAX_REPORT_BYTES",8192,256,65536),BindingsFile:bindings}
	if err:=validateControlPlaneURL(s.ControlPlaneURL,s.AllowInsecureHTTP);err!=nil{return Settings{},fmt.Errorf("workforce control plane: %w",err)}
	return s,nil
}

func filepathJoin(parts ...string) string { return strings.Join(parts,string(os.PathSeparator)) }

func NewClient(settings Settings,version string)(*Client,error){if !settings.Enabled{return nil,errors.New("workforce client disabled")};if !serviceTokenPattern.MatchString(settings.ServiceToken){return nil,errors.New("invalid workforce service token")};if err:=validateControlPlaneURL(settings.ControlPlaneURL,settings.AllowInsecureHTTP);err!=nil{return nil,err};base,_:=url.Parse(strings.TrimRight(settings.ControlPlaneURL,"/"));return &Client{base:base,token:settings.ServiceToken,version:version,http:&http.Client{Timeout:settings.RequestTimeout,CheckRedirect:func(_ *http.Request,_ []*http.Request)error{return http.ErrUseLastResponse}}},nil}
func (c *Client) Heartbeat(ctx context.Context,capabilities []string)error{var out struct{OK bool `json:"ok"`};return c.doJSON(ctx,http.MethodPost,"/api/v1/workforce/runtime/heartbeat",map[string]any{"version":c.version,"platform":fmt.Sprintf("%s/%s",runtimeGOOS(),runtimeGOARCH()),"capabilities":capabilities},&out)}
func runtimeGOOS() string { return os.Getenv("GOOS") }
func runtimeGOARCH() string { return os.Getenv("GOARCH") }
func (c *Client) Sync(ctx context.Context)(*SyncResponse,error){var out SyncResponse;if err:=c.doJSON(ctx,http.MethodGet,"/api/v1/workforce/runtime/sync",nil,&out);err!=nil{return nil,err};if !out.OK{return nil,errors.New("workforce sync not acknowledged")};if out.Policy.ArbitraryShell||out.Policy.GenericRemoteShell||!out.Policy.CloudNeverBypassesLocalApproval||out.Policy.CredentialsInCloud||out.Policy.ConnectorConfigGrantsPermission{return nil,errors.New("unsafe workforce cloud policy rejected")};if out.Schema!="kingai.workforce.v2"{return nil,fmt.Errorf("unsupported workforce schema %q",out.Schema)};return &out,nil}
func (c *Client) PullTask(ctx context.Context)(*CloudTask,error){var out struct{OK bool `json:"ok"`;Task *CloudTask `json:"task"`};if err:=c.doJSON(ctx,http.MethodPost,"/api/v1/workforce/runtime/tasks/pull",map[string]any{},&out);err!=nil{return nil,err};return out.Task,nil}
func (c *Client) ReportResult(ctx context.Context,taskID,status string,output any,errorText string)error{if taskID==""{return errors.New("cloud task id required")};if status!="succeeded"&&status!="failed"{return errors.New("invalid cloud task result status")};body:=map[string]any{"status":status};if output!=nil{body["output"]=output};if errorText!=""{body["error"]=errorText};var out struct{OK bool `json:"ok"`};return c.doJSON(ctx,http.MethodPost,"/api/v1/workforce/runtime/tasks/"+url.PathEscape(taskID)+"/result",body,&out)}
func (c *Client) doJSON(ctx context.Context,method,path string,input,output any)error{ref,err:=url.Parse(path);if err!=nil{return err};target:=c.base.ResolveReference(ref);if !strings.EqualFold(target.Hostname(),c.base.Hostname())||target.Scheme!=c.base.Scheme{return errors.New("workforce request escaped configured control-plane origin")};var body io.Reader;if input!=nil{encoded,err:=json.Marshal(input);if err!=nil{return err};if len(encoded)>128<<10{return errors.New("workforce request body exceeds limit")};body=bytes.NewReader(encoded)};req,err:=http.NewRequestWithContext(ctx,method,target.String(),body);if err!=nil{return err};req.Header.Set("Authorization","Bearer "+c.token);req.Header.Set("Accept","application/json");req.Header.Set("User-Agent","KINGAIBOT/"+c.version+" enterprise-workforce-v2");if input!=nil{req.Header.Set("Content-Type","application/json")};resp,err:=c.http.Do(req);if err!=nil{return err};defer resp.Body.Close();payload,err:=io.ReadAll(io.LimitReader(resp.Body,(1<<20)+1));if err!=nil{return err};if len(payload)>1<<20{return errors.New("workforce response exceeds limit")};if resp.StatusCode<200||resp.StatusCode>299{return fmt.Errorf("workforce API returned HTTP %d",resp.StatusCode)};if output!=nil&&len(payload)>0{if err:=json.Unmarshal(payload,output);err!=nil{return fmt.Errorf("invalid workforce response: %w",err)}};return nil}
func validateControlPlaneURL(raw string,allowHTTP bool)error{u,err:=url.Parse(raw);if err!=nil{return err};if u.Hostname()==""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""{return errors.New("URL requires hostname and must not contain credentials, query or fragment")};if u.Scheme=="https"{return nil};if u.Scheme!="http"||!allowHTTP{return errors.New("control plane must use https")};host:=u.Hostname();if strings.EqualFold(host,"localhost"){return nil};if ip:=net.ParseIP(host);ip!=nil&&ip.IsLoopback(){return nil};return errors.New("insecure http allowed only for loopback development")}
func envBool(name string)bool{v:=strings.ToLower(strings.TrimSpace(os.Getenv(name)));return v=="1"||v=="true"||v=="yes"||v=="on"}
func envDuration(name string,def,min,max time.Duration)time.Duration{return time.Duration(envInt(name,int(def/time.Second),int(min/time.Second),int(max/time.Second)))*time.Second}
func envInt(name string,def,min,max int)int{v:=strings.TrimSpace(os.Getenv(name));if v==""{return def};var n int;if _,err:=fmt.Sscanf(v,"%d",&n);err!=nil{return def};if n<min{return min};if n>max{return max};return n}
