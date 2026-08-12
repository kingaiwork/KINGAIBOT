package runtime

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/agent"
	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/evolution"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/policy"
	"github.com/kingaiwork/KINGAIBOT/internal/provider"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
	"github.com/kingaiwork/KINGAIBOT/internal/tool"
)

func TestCancelRunningTaskCannotBeOverwrittenByWorker(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select { case started <- struct{}{}: default: }
		select { case <-r.Context().Done(): return; case <-release: w.Header().Set("Content-Type", "application/json"); _, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"should-not-win"},"finish_reason":"stop"}]}`)) }
	}))
	root := t.TempDir()
	cfg := &config.Config{Runtime: config.Runtime{DataDir:root,WorkspaceDir:filepath.Join(root,"workspace"),MaxSteps:1,WorkerCount:1,RequestTimeoutSeconds:30,TaskTimeoutSeconds:30,QueueCapacity:8,MaxRequestBytes:1<<20},Memory:config.Memory{Enabled:false,MaxRecords:100,MaxContextChars:1000},Providers:[]config.Provider{{Name:"test",BaseURL:upstream.URL,Model:"test-model",Enabled:true,AllowPrivateNetwork:true,AllowInsecureHTTP:true}},Security:config.Security{DefaultToolPolicy:"deny",ToolPolicies:map[string]string{}},Evolution:config.Evolution{Enabled:false,Mode:"proposal-only"}}
	if err:=cfg.Normalize(root);err!=nil{t.Fatal(err)}
	ts,err:=task.NewStore(filepath.Join(root,"tasks"));if err!=nil{t.Fatal(err)}
	as,err:=approval.New(filepath.Join(root,"approvals"));if err!=nil{t.Fatal(err)}
	el,err:=eventlog.New(filepath.Join(root,"events"));if err!=nil{t.Fatal(err)}
	ms,err:=memory.New(filepath.Join(root,"memory"),100);if err!=nil{t.Fatal(err)}
	es,err:=evolution.New(filepath.Join(root,"evolution"));if err!=nil{t.Fatal(err)}
	pe:=policy.New("deny",nil);tr:=tool.New(cfg,pe,as,el);pc:=provider.New(cfg.Providers,30*time.Second);ae:=agent.New(cfg,pc,tr,ms);rt:=New(ts,as,el,ms,ae,es,cfg)
	created,err:=rt.Create("block until canceled",nil);if err!=nil{t.Fatal(err)}
	select{case<-started:case<-time.After(3*time.Second):t.Fatal("provider request never started")}
	if err:=rt.Cancel(created.ID);err!=nil{t.Fatal(err)}
	deadline:=time.Now().Add(3*time.Second);canceled:=false
	for time.Now().Before(deadline){got,err:=rt.Task(created.ID);if err!=nil{t.Fatal(err)};if got.Status==task.Canceled{canceled=true;break};time.Sleep(20*time.Millisecond)}
	if !canceled{close(release);rt.Close();upstream.Close();t.Fatal("task did not enter canceled state")}
	close(release);time.Sleep(150*time.Millisecond);got2,_:=rt.Task(created.ID);if got2.Status!=task.Canceled{t.Fatalf("worker overwrote canceled state with %s",got2.Status)};rt.Close();upstream.Close()
}
