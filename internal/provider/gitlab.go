package provider

import (
	"context"
	"fmt"
	"net/url"
	"roadmap-tool/internal/model"
	"time"
)

type GitLab struct {
	c       *client
	project string
}

func NewGitLab(baseURL, token, owner, repo string) *GitLab {
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	return &GitLab{newClient(baseURL, token), url.PathEscape(owner + "/" + repo)}
}

type gitlabIssue struct {
	IID       int       `json:"iid"`
	Title     string    `json:"title"`
	WebURL    string    `json:"web_url"`
	State     string    `json:"state"`
	Labels    []string  `json:"labels"`
	CreatedAt time.Time `json:"created_at"`
	DueDate   *string   `json:"due_date"`
	Milestone *struct {
		ID int `json:"id"`
	} `json:"milestone"`
}
type gitlabMilestone struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	WebURL      string  `json:"web_url"`
	State       string  `json:"state"`
	DueDate     *string `json:"due_date"`
}

func (g *GitLab) headers() map[string]string {
	h := map[string]string{"Accept": "application/json"}
	if g.c.token != "" {
		h["PRIVATE-TOKEN"] = g.c.token
	}
	return h
}
func parseDatePtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, e := time.Parse("2006-01-02", *s)
	if e != nil {
		return nil
	}
	return &t
}
func (g *GitLab) Issues(ctx context.Context) ([]model.Issue, error) {
	var raw []gitlabIssue
	u := fmt.Sprintf("%s/api/v4/projects/%s/issues?scope=all&state=all&per_page=100", g.c.baseURL, g.project)
	if err := g.c.get(u, g.headers(), &raw); err != nil {
		return nil, err
	}
	out := make([]model.Issue, 0, len(raw))
	for _, x := range raw {
		milestoneID := ""
		if x.Milestone != nil {
			milestoneID = fmt.Sprint(x.Milestone.ID)
		}
		out = append(out, model.Issue{ID: fmt.Sprint(x.IID), Title: x.Title, URL: x.WebURL, Labels: x.Labels, State: x.State, MilestoneID: milestoneID, DueDate: parseDatePtr(x.DueDate)})
	}
	return out, nil
}
func (g *GitLab) Milestones(ctx context.Context) ([]model.Milestone, error) {
	var raw []gitlabMilestone
	u := fmt.Sprintf("%s/api/v4/projects/%s/milestones?state=all&per_page=100", g.c.baseURL, g.project)
	if err := g.c.get(u, g.headers(), &raw); err != nil {
		return nil, err
	}
	out := []model.Milestone{}
	for _, x := range raw {
		out = append(out, model.Milestone{ID: fmt.Sprint(x.ID), Title: x.Title, Description: x.Description, URL: x.WebURL, State: x.State, Due: parseDatePtr(x.DueDate)})
	}
	return out, nil
}
