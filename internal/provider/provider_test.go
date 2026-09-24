package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer(t *testing.T, issuePath, issueJSON, milestonePath, milestoneJSON string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case issuePath:
			fmt.Fprint(w, issueJSON)
		case milestonePath:
			fmt.Fprint(w, milestoneJSON)
		default:
			http.NotFound(w, r)
		}
	}))
}

func assertMapping(t *testing.T, p Provider, wantMilestoneID, wantDescription string, wantIssueDue bool) {
	t.Helper()
	issues, err := p.Issues(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].MilestoneID != wantMilestoneID || !strings.EqualFold(issues[0].Labels[0], "feature") {
		t.Fatalf("unexpected issues: %#v", issues)
	}
	if (issues[0].DueDate != nil) != wantIssueDue {
		t.Fatalf("issue DueDate present = %v, want %v", issues[0].DueDate != nil, wantIssueDue)
	}
	milestones, err := p.Milestones(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(milestones) != 1 || milestones[0].ID != wantMilestoneID || milestones[0].Description != wantDescription || milestones[0].State != "open" || milestones[0].Due == nil {
		t.Fatalf("unexpected milestones: %#v", milestones)
	}
}

func TestGitHubMapsIssueMilestoneAndDescription(t *testing.T) {
	s := testServer(t,
		"/repos/acme/product/issues",
		`[{"number":1,"title":"Feature","html_url":"https://example.test/i/1","state":"open","labels":[{"name":"feature"}],"created_at":"2026-01-01T00:00:00Z","milestone":{"number":7}}]`,
		"/repos/acme/product/milestones",
		`[{"number":7,"title":"v1","description":"GitHub release","html_url":"https://example.test/m/7","state":"open","due_on":"2026-02-01T00:00:00Z"}]`)
	defer s.Close()
	assertMapping(t, NewGitHub(s.URL, "", "acme", "product"), "7", "GitHub release", false)
}

func TestGitLabMapsIssueMilestoneAndDescription(t *testing.T) {
	s := testServer(t,
		"/api/v4/projects/acme/product/issues",
		`[{"iid":1,"title":"Feature","web_url":"https://example.test/i/1","state":"opened","labels":["feature"],"created_at":"2026-01-01T00:00:00Z","due_date":"2026-01-20","milestone":{"id":8}}]`,
		"/api/v4/projects/acme/product/milestones",
		`[{"id":8,"title":"v1","description":"GitLab release","web_url":"https://example.test/m/8","state":"open","due_date":"2026-02-01"}]`)
	defer s.Close()
	assertMapping(t, NewGitLab(s.URL, "", "acme", "product"), "8", "GitLab release", true)
}

func TestGiteaMapsIssueMilestoneAndDescription(t *testing.T) {
	s := testServer(t,
		"/api/v1/repos/acme/product/issues",
		`[{"number":1,"title":"Feature","html_url":"https://example.test/i/1","state":"open","labels":[{"name":"feature"}],"created_at":"2026-01-01T00:00:00Z","due_date":"2026-01-20T00:00:00Z","milestone":{"id":9}}]`,
		"/api/v1/repos/acme/product/milestones",
		`[{"id":9,"title":"v1","description":"Gitea release","html_url":"https://example.test/m/9","state":"open","due_on":"2026-02-01T00:00:00Z"}]`)
	defer s.Close()
	assertMapping(t, NewGitea(s.URL, "", "acme", "product"), "9", "Gitea release", true)
}
