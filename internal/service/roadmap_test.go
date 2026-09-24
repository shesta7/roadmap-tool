package service

import (
	"context"
	"testing"
	"time"

	"roadmap-tool/internal/model"
)

type fakeProvider struct {
	issues     []model.Issue
	milestones []model.Milestone
}

func (f fakeProvider) Issues(context.Context) ([]model.Issue, error) { return f.issues, nil }
func (f fakeProvider) Milestones(context.Context) ([]model.Milestone, error) {
	return f.milestones, nil
}

func TestLoadGroupsOnlyLabeledIssuesAndSkipsUndatedMilestones(t *testing.T) {
	early := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	issueDue := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	p := fakeProvider{
		issues: []model.Issue{
			{ID: "1", Title: "Included", Labels: []string{"TYPE::FEATURE"}, MilestoneID: "early", DueDate: &issueDue},
			{ID: "2", Title: "Wrong label", Labels: []string{"bug"}, MilestoneID: "early"},
			{ID: "3", Title: "No milestone", Labels: []string{"type::feature"}},
		},
		milestones: []model.Milestone{
			{ID: "late", Title: "Late", Due: &late},
			{ID: "undated", Title: "Undated"},
			{ID: "early", Title: "Early", Due: &early},
		},
	}

	got, err := New(p, "type::feature").Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.SkippedMilestones != 1 {
		t.Fatalf("SkippedMilestones = %d, want 1", got.SkippedMilestones)
	}
	if len(got.Milestones) != 2 || got.Milestones[0].ID != "early" || got.Milestones[1].ID != "late" {
		t.Fatalf("milestones not sorted or filtered: %#v", got.Milestones)
	}
	if len(got.Milestones[0].Issues) != 1 || got.Milestones[0].Issues[0].ID != "1" {
		t.Fatalf("unexpected grouped issues: %#v", got.Milestones[0].Issues)
	}
	if !got.MinDate.Equal(issueDue) {
		t.Fatalf("MinDate = %s, want issue due date %s", got.MinDate, issueDue)
	}
}

func TestLoadExpandsRangeForSingleDate(t *testing.T) {
	due := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	got, err := New(fakeProvider{milestones: []model.Milestone{{ID: "1", Due: &due}}}, "feature").Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.MinDate.Before(due) || !got.MaxDate.After(due) {
		t.Fatalf("single milestone range was not expanded: %s - %s", got.MinDate, got.MaxDate)
	}
}
