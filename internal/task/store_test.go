package task

import "testing"

func TestStoreRejectsTraversalID(t *testing.T) { s,err:=NewStore(t.TempDir());if err!=nil{t.Fatal(err)};if err:=s.Save(&Task{ID:"../escape",Status:Queued});err==nil{t.Fatal("expected traversal id to be rejected")};if _,err:=s.Get("../escape");err==nil{t.Fatal("expected traversal get to be rejected")} }
func TestCancelCannotOverwriteTerminalTask(t *testing.T) { s,err:=NewStore(t.TempDir());if err!=nil{t.Fatal(err)};if err:=s.Save(&Task{ID:"task_1",Status:Completed});err!=nil{t.Fatal(err)};if err:=s.Cancel("task_1");err==nil{t.Fatal("expected terminal cancel to fail")};got,err:=s.Get("task_1");if err!=nil{t.Fatal(err)};if got.Status!=Completed{t.Fatalf("terminal status changed: %s",got.Status)} }
