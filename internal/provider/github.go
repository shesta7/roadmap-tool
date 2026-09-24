package provider

import (
	"context"
	"fmt"
	"roadmap-tool/internal/model"
	"time"
)

type GitHub struct {
	c           *client
	owner, repo string
}

func NewGitHub(baseURL, token, owner, repo string) *GitHub {
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return &GitHub{newClient(baseURL, token), owner, repo}
}

type githubLabel struct {
	Name string `json:"name"`
}
type githubMilestone struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	HTMLURL     string     `json:"html_url"`
	State       string     `json:"state"`
	DueOn       *time.Time `json:"due_on"`
}
type githubIssue struct {
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	HTMLURL   string        `json:"html_url"`
	State     string        `json:"state"`
	Labels    []githubLabel `json:"labels"`
	CreatedAt time.Time     `json:"created_at"`
	Milestone *struct {
		Number int `json:"number"`
	} `json:"milestone"`
	PullRequest any `json:"pull_request"`
}

func (g *GitHub) headers() map[string]string {
	h := map[string]string{"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"}
	if g.c.token != "" {
		h["Authorization"] = "Bearer " + g.c.token
	}
	return h
}
func (g *GitHub) Issues(ctx context.Context) ([]model.Issue, error) {
	var raw []githubIssue
	u := fmt.Sprintf("%s/repos/%s/%s/issues?state=all&per_page=100", g.c.baseURL, g.owner, g.repo)
	if err := g.c.get(u, g.headers(), &raw); err != nil {
		return nil, err
	}
	out := []model.Issue{}
	for _, x := range raw {
		if x.PullRequest != nil {
			continue
		}
		labels := []string{}
		for _, l := range x.Labels {
			labels = append(labels, l.Name)
		}
		milestoneID := ""
		if x.Milestone != nil {
			milestoneID = fmt.Sprint(x.Milestone.Number)
		}
		out = append(out, model.Issue{ID: fmt.Sprint(x.Number), Title: x.Title, URL: x.HTMLURL, Labels: labels, State: x.State, MilestoneID: milestoneID})
	}
	return out, nil
}
func (g *GitHub) Milestones(ctx context.Context) ([]model.Milestone, error) {
	var raw []githubMilestone
	u := fmt.Sprintf("%s/repos/%s/%s/milestones?state=all&per_page=100", g.c.baseURL, g.owner, g.repo)
	if err := g.c.get(u, g.headers(), &raw); err != nil {
		return nil, err
	}
	out := []model.Milestone{}
	for _, x := range raw {
		out = append(out, model.Milestone{ID: fmt.Sprint(x.Number), Title: x.Title, Description: x.Description, URL: x.HTMLURL, State: x.State, Due: x.DueOn})
	}
	return out, nil
}
