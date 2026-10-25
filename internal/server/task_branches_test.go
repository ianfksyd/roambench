package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
)

// P3: 分支派生与成果综合的事务接口。
func seedP3Task(t *testing.T, srv *Server, token, title string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks",
		strings.NewReader(`{"projectId":"project-roambench-control","workstreamId":"workstream-ui-ia","title":"`+title+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task = %d: %s", rec.Code, rec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, rec)
	for _, task := range snapshot.Tasks {
		if task.Title == title {
			return task.ID
		}
	}
	t.Fatal("task not found")
	return ""
}

func TestP3BranchesCreateDependentChildren(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	parentID := seedP3Task(t, srv, token, "P3 父任务")

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks/"+parentID+"/branches",
		strings.NewReader(`{"branches":[{"title":"审查线 1"},{"title":"审查线 2","goal":"深入核查"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("branches = %d: %s", rec.Code, rec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, rec)

	branchCount := 0
	for _, task := range snapshot.Tasks {
		if len(task.DependsOn) == 1 && task.DependsOn[0] == parentID {
			branchCount++
			if task.Origin != "human" {
				t.Errorf("branch origin = %q, want human", task.Origin)
			}
		}
	}
	if branchCount != 2 {
		t.Fatalf("branch tasks = %d, want 2", branchCount)
	}
}

func TestP3BranchesAtomicFailure(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	parentID := seedP3Task(t, srv, token, "P3 原子父任务")

	// 第二条分支标题为空 → 整批失败，第一条也不应存在。
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks/"+parentID+"/branches",
		strings.NewReader(`{"branches":[{"title":"好的分支"},{"title":""}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("invalid batch should fail")
	}
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range state.Tasks {
		if task.Title == "好的分支" {
			t.Fatal("partial branch created despite batch failure")
		}
	}
}

func TestP3ComposeWaitsForAllSources(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == "task-information-architecture" {
				state.Tasks[i].State = "planned"
				state.Tasks[i].DispatchState = "done"
				state.Tasks[i].AcceptanceCommand = nil
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sourceID := "task-information-architecture"

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks/"+sourceID+"/compose",
		strings.NewReader(`{"sourceTaskIds":["`+sourceID+`"],"title":"综合结论","goal":"基于两份成果综合"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compose = %d: %s", rec.Code, rec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, rec)
	composeID := ""
	for _, task := range snapshot.Tasks {
		if task.Title == "综合结论" {
			composeID = task.ID
			if len(task.DependsOn) != 1 || task.DependsOn[0] != sourceID {
				t.Errorf("compose DependsOn = %v, want [%s]", task.DependsOn, sourceID)
			}
		}
	}
	if composeID == "" {
		t.Fatal("composed task not found")
	}
}

func TestP3ComposeRejectsUndoneSource(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)
	if _, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Tasks = append(state.Tasks, projectControlTask{
			ID: "task-p3-source", ProjectID: "project-roambench-control",
			Title: "进行中的来源", State: "running", DispatchState: "claimed",
			Timeline: []projectControlEvent{}, Audit: []projectControlAuditItem{},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks/task-p3-source/compose",
		strings.NewReader(`{"sourceTaskIds":["task-p3-source"],"title":"太早"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("compose from a running source must fail")
	}
}

func TestComposedTaskPersistsAllSourceDescriptions(t *testing.T) {
	srv, token, sessions := progressServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)
	ids := []string{seedP3Task(t, srv, token, "first source"), seedP3Task(t, srv, token, "second source")}
	_, err := srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == ids[0] || state.Tasks[i].ID == ids[1] {
				state.Tasks[i].DispatchState = "done"
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(taskComposeRequest{SourceTaskIDs: ids, Title: "combined result"})
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/tasks/"+ids[0]+"/compose", strings.NewReader(string(body)))
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compose: %d %s", rec.Code, rec.Body.String())
	}
	state, err := srv.projectControl.loadOrSeed("ian")
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range state.Tasks {
		if task.Title == "combined result" {
			if task.Description != "Synthesized from: ["+strings.Join(ids, ", ")+"]" || len(task.DependsOn) != 2 {
				t.Fatalf("provenance lost: %+v", task)
			}
			return
		}
	}
	t.Fatal("composed task not persisted")
}

// 依赖 PATCH 不得制造循环：A 依赖 B 时，把 B 改为依赖 A 必须被拒绝。
func TestP3CycleRejectedOnDependencyPatch(t *testing.T) {
	srv, token, sessions := testProjectControlServer(t)
	defer sessions.Stop()
	muteSeedTasks(t, srv)

	taskA := seedP3Task(t, srv, token, "P3 循环 A")
	taskB := seedP3Task(t, srv, token, "P3 循环 B")

	// B 依赖 A：合法。
	patch := func(taskID, deps string) int {
		req := httptest.NewRequest(http.MethodPatch, "/api/project-control/tasks/"+taskID,
			strings.NewReader(`{"expectedRowVersion":1,"action":"queue_task","dependsOn":`+deps+`}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := patch(taskB, `["`+taskA+`"]`); code >= 400 {
		t.Fatalf("B depends on A = %d, want success", code)
	}
	// A 依赖 B → A→B→A 循环：必须拒绝。
	if code := patch(taskA, `["`+taskB+`"]`); code < 400 {
		t.Fatalf("cyclic dependency PATCH = %d, want rejection", code)
	}
}
