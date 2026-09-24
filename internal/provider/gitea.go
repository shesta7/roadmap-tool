package provider

import (
	"context"
	"fmt"
	"roadmap-tool/internal/model"
	"time"
)

type Gitea struct {
	c           *client
	owner, repo string
}

func NewGitea(baseURL, token, owner, repo string) *Gitea {
	return &Gitea{newClient(baseURL, token), owner, repo}
}

type giteaLabel struct {
	Name string `json:"name"`
}
type giteaMilestone struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueOn       *time.Time `json:"due_on"`
	HTMLURL     string     `json:"html_url"`
	State       string     `json:"state"`
}
type giteaIssue struct {
	ID        int64        `json:"id"`
	Number    int64        `json:"number"`
	Title     string       `json:"title"`
	HTMLURL   string       `json:"html_url"`
	State     string       `json:"state"`
	Labels    []giteaLabel `json:"labels"`
	DueDate   *time.Time   `json:"due_date"`
	CreatedAt time.Time    `json:"created_at"`
	ClosedAt  *time.Time   `json:"closed_at"`
	Milestone *struct {
		ID int64 `json:"id"`
	} `json:"milestone"`
}

func (g *Gitea) headers() map[string]string {
	h := map[string]string{"Accept": "application/json"}
	if g.c.token != "" {
		h["Authorization"] = "token " + g.c.token
	}
	return h
}
func (g *Gitea) Issues(ctx context.Context) ([]model.Issue, error) {
	var raw []giteaIssue
	u := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues?state=all&limit=100", g.c.baseURL, g.owner, g.repo)
	if err := g.c.get(u, g.headers(), &raw); err != nil {
		return nil, err
	}
	out := make([]model.Issue, 0, len(raw))
	for _, x := range raw {
		labels := make([]string, 0, len(x.Labels))
		for _, l := range x.Labels {
			labels = append(labels, l.Name)
		}
		milestoneID := ""
		if x.Milestone != nil {
			milestoneID = fmt.Sprint(x.Milestone.ID)
		}
		out = append(out, model.Issue{
			ID:          fmt.Sprint(x.Number),
			Title:       x.Title,
			URL:         x.HTMLURL,
			Labels:      labels,
			State:       x.State,
			MilestoneID: milestoneID,
			DueDate:     x.DueDate,
		})
	}
	return out, nil
}
func (g *Gitea) Milestones(ctx context.Context) ([]model.Milestone, error) {
	var raw []giteaMilestone
	u := fmt.Sprintf("%s/api/v1/repos/%s/%s/milestones?state=all&limit=100", g.c.baseURL, g.owner, g.repo)
	if err := g.c.get(u, g.headers(), &raw); err != nil {
		return nil, err
	}
	out := []model.Milestone{}
	for _, x := range raw {
		out = append(out, model.Milestone{ID: fmt.Sprint(x.ID), Title: x.Title, Description: x.Description, URL: x.HTMLURL, State: x.State, Due: x.DueOn})
	}
	return out, nil
}
