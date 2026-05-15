package github

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	gh "github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

type Client struct {
	gh *gh.Client
}

func NewClient(ctx context.Context) (*Client, error) {
	token, err := getGHToken()
	if err != nil {
		return nil, fmt.Errorf("github auth: %w (is `gh` CLI installed and authenticated?)", err)
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, ts)
	return &Client{gh: gh.NewClient(httpClient)}, nil
}

func getGHToken() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("running `gh auth token`: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *Client) FetchWorkItems(ctx context.Context, owner, repo string, labels []string) ([]WorkItem, error) {
	var items []WorkItem

	// Fetch issues
	issueOpts := &gh.IssueListByRepoOptions{
		State:     "open",
		Sort:      "updated",
		Direction: "desc",
		Labels:    labels,
		ListOptions: gh.ListOptions{PerPage: 50},
	}
	issues, _, err := c.gh.Issues.ListByRepo(ctx, owner, repo, issueOpts)
	if err != nil {
		return nil, fmt.Errorf("listing issues for %s/%s: %w", owner, repo, err)
	}
	for _, issue := range issues {
		if issue.IsPullRequest() {
			continue
		}
		items = append(items, issueToWorkItem(owner, repo, issue))
	}

	// Fetch PRs
	prOpts := &gh.PullRequestListOptions{
		State:     "open",
		Sort:      "updated",
		Direction: "desc",
		ListOptions: gh.ListOptions{PerPage: 50},
	}
	prs, _, err := c.gh.PullRequests.List(ctx, owner, repo, prOpts)
	if err != nil {
		return nil, fmt.Errorf("listing PRs for %s/%s: %w", owner, repo, err)
	}
	for _, pr := range prs {
		items = append(items, prToWorkItem(owner, repo, pr))
	}

	return items, nil
}

func (c *Client) FetchComments(ctx context.Context, owner, repo string, number int) ([]Comment, error) {
	ghComments, _, err := c.gh.Issues.ListComments(ctx, owner, repo, number, &gh.IssueListCommentsOptions{
		Sort:        gh.String("created"),
		Direction:   gh.String("asc"),
		ListOptions: gh.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, err
	}
	var comments []Comment
	for _, c := range ghComments {
		comments = append(comments, Comment{
			Author:    c.GetUser().GetLogin(),
			Body:      c.GetBody(),
			CreatedAt: c.GetCreatedAt().Time,
		})
	}
	return comments, nil
}

func (c *Client) FetchPRDiff(ctx context.Context, owner, repo string, number int) (string, error) {
	diff, _, err := c.gh.PullRequests.GetRaw(ctx, owner, repo, number, gh.RawOptions{Type: gh.Diff})
	if err != nil {
		return "", fmt.Errorf("fetching diff for %s/%s#%d: %w", owner, repo, number, err)
	}
	return diff, nil
}

func (c *Client) FetchIssueState(ctx context.Context, owner, repo string, number int) (string, error) {
	issue, _, err := c.gh.Issues.Get(ctx, owner, repo, number)
	if err != nil {
		return "", fmt.Errorf("fetching issue %s/%s#%d: %w", owner, repo, number, err)
	}
	return issue.GetState(), nil
}

func (c *Client) FetchPRState(ctx context.Context, owner, repo string, number int) (string, error) {
	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return "", fmt.Errorf("fetching PR %s/%s#%d: %w", owner, repo, number, err)
	}
	if pr.GetMerged() {
		return "merged", nil
	}
	return pr.GetState(), nil
}

func issueToWorkItem(owner, repo string, issue *gh.Issue) WorkItem {
	var labels []string
	for _, l := range issue.Labels {
		labels = append(labels, l.GetName())
	}
	return WorkItem{
		Kind:      KindIssue,
		Owner:     owner,
		Repo:      repo,
		Number:    issue.GetNumber(),
		Title:     issue.GetTitle(),
		Body:      issue.GetBody(),
		Author:    issue.GetUser().GetLogin(),
		Labels:    labels,
		State:     issue.GetState(),
		UpdatedAt: issue.GetUpdatedAt().Time,
	}
}

func prToWorkItem(owner, repo string, pr *gh.PullRequest) WorkItem {
	var labels []string
	for _, l := range pr.Labels {
		labels = append(labels, l.GetName())
	}
	state := pr.GetState()
	if pr.GetMerged() {
		state = "merged"
	}
	return WorkItem{
		Kind:      KindPR,
		Owner:     owner,
		Repo:      repo,
		Number:    pr.GetNumber(),
		Title:     pr.GetTitle(),
		Body:      pr.GetBody(),
		Author:    pr.GetUser().GetLogin(),
		Labels:    labels,
		State:     state,
		HeadRef:   pr.GetHead().GetRef(),
		UpdatedAt: pr.GetUpdatedAt().Time,
	}
}
