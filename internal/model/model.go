package model

import "time"

type Issue struct {
	ID          string
	Title       string
	URL         string
	Labels      []string
	State       string
	MilestoneID string
	DueDate     *time.Time
}

type Milestone struct {
	ID          string
	Title       string
	Description string
	URL         string
	State       string
	Due         *time.Time
	Issues      []Issue
}

type Roadmap struct {
	Milestones        []Milestone
	MinDate           time.Time
	MaxDate           time.Time
	SkippedMilestones int
}
