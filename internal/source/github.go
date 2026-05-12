package source

import (
	"context"
	"fmt"

	ghclient "github.com/Cali0707/baton/internal/github"
	"github.com/Cali0707/baton/internal/store"
)

// GitHubSource implements Source by fetching from the GitHub API.
type GitHubSource struct {
	client *ghclient.Client
}

// NewGitHub creates a Source backed by the GitHub API.
func NewGitHub(client *ghclient.Client) *GitHubSource {
	return &GitHubSource{client: client}
}

func (g *GitHubSource) FetchDetail(ctx context.Context, item *store.InboxItem) (*ItemDetail, error) {
	if item.Number == nil {
		return &ItemDetail{}, nil
	}
	number := *item.Number

	comments, err := g.client.FetchComments(ctx, item.Owner, item.Repo, number)
	if err != nil {
		return nil, fmt.Errorf("fetching comments for %s/%s#%d: %w", item.Owner, item.Repo, number, err)
	}

	detail := &ItemDetail{}
	for _, c := range comments {
		detail.Comments = append(detail.Comments, Comment{
			Author:    c.Author,
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
		})
	}

	if item.Kind == "pr" {
		diff, err := g.client.FetchPRDiff(ctx, item.Owner, item.Repo, number)
		if err != nil {
			return nil, fmt.Errorf("fetching diff for %s/%s#%d: %w", item.Owner, item.Repo, number, err)
		}
		detail.Diff = diff
	}

	return detail, nil
}
