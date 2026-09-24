package provider

import (
	"context"
	"roadmap-tool/internal/model"
)

type Provider interface {
	Issues(ctx context.Context) ([]model.Issue, error)
	Milestones(ctx context.Context) ([]model.Milestone, error)
}
