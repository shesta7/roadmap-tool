package service

import (
	"context"
	"roadmap-tool/internal/model"
	"roadmap-tool/internal/provider"
	"sort"
	"strings"
	"time"
)

type Service struct {
	p            provider.Provider
	featureLabel string
}

func New(p provider.Provider, featureLabel string) *Service {
	return &Service{p: p, featureLabel: featureLabel}
}
func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if strings.EqualFold(l, want) {
			return true
		}
	}
	return false
}
func (s *Service) Load(ctx context.Context) (model.Roadmap, error) {
	issues, err := s.p.Issues(ctx)
	if err != nil {
		return model.Roadmap{}, err
	}
	ms, err := s.p.Milestones(ctx)
	if err != nil {
		return model.Roadmap{}, err
	}
	byMilestone := make(map[string][]model.Issue)
	for _, x := range issues {
		if x.MilestoneID != "" && hasLabel(x.Labels, s.featureLabel) {
			byMilestone[x.MilestoneID] = append(byMilestone[x.MilestoneID], x)
		}
	}

	roadmap := model.Roadmap{Milestones: []model.Milestone{}}
	for _, m := range ms {
		if m.Due == nil {
			roadmap.SkippedMilestones++
			continue
		}
		m.Issues = append([]model.Issue{}, byMilestone[m.ID]...)
		sort.Slice(m.Issues, func(i, j int) bool { return m.Issues[i].Title < m.Issues[j].Title })
		roadmap.Milestones = append(roadmap.Milestones, m)
	}
	sort.Slice(roadmap.Milestones, func(i, j int) bool {
		return roadmap.Milestones[i].Due.Before(*roadmap.Milestones[j].Due)
	})

	if len(roadmap.Milestones) == 0 {
		now := time.Now().UTC()
		roadmap.MinDate = now.AddDate(0, -1, 0)
		roadmap.MaxDate = now.AddDate(0, 1, 0)
		return roadmap, nil
	}
	roadmap.MinDate = *roadmap.Milestones[0].Due
	roadmap.MaxDate = *roadmap.Milestones[len(roadmap.Milestones)-1].Due
	for _, m := range roadmap.Milestones {
		for _, issue := range m.Issues {
			if issue.DueDate == nil {
				continue
			}
			if issue.DueDate.Before(roadmap.MinDate) {
				roadmap.MinDate = *issue.DueDate
			}
			if issue.DueDate.After(roadmap.MaxDate) {
				roadmap.MaxDate = *issue.DueDate
			}
		}
	}
	if !roadmap.MaxDate.After(roadmap.MinDate) {
		roadmap.MinDate = roadmap.MinDate.AddDate(0, -1, 0)
		roadmap.MaxDate = roadmap.MaxDate.AddDate(0, 1, 0)
	}
	return roadmap, nil
}
