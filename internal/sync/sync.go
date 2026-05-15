package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ghclient "github.com/Cali0707/baton/internal/github"
	"github.com/Cali0707/baton/internal/config"
	"github.com/Cali0707/baton/internal/store"
)

// Syncer bridges GitHub (and future sources) with the local database.
type Syncer struct {
	db       store.Store
	ghClient *ghclient.Client
	repos    []config.RepoConfig
}

func New(db store.Store, ghClient *ghclient.Client, repos []config.RepoConfig) SyncService {
	return &Syncer{db: db, ghClient: ghClient, repos: repos}
}

// Sync fetches from all configured sources and upserts into the DB.
func (s *Syncer) Sync(ctx context.Context) (newCount, updatedCount int, err error) {
	for _, repo := range s.repos {
		n, u, syncErr := s.syncRepo(ctx, repo)
		if syncErr != nil {
			return newCount, updatedCount, fmt.Errorf("syncing %s/%s: %w", repo.Owner, repo.Name, syncErr)
		}
		newCount += n
		updatedCount += u
	}
	return newCount, updatedCount, nil
}

func (s *Syncer) syncRepo(ctx context.Context, repo config.RepoConfig) (newCount, updatedCount int, err error) {
	workItems, err := s.ghClient.FetchWorkItems(ctx, repo.Owner, repo.Name, repo.Labels)
	if err != nil {
		return 0, 0, err
	}

	fetchedSourceIDs := make(map[string]bool, len(workItems))
	for _, wi := range workItems {
		item := workItemToInboxItem(wi)
		fetchedSourceIDs[item.SourceID] = true
		existing, _ := s.db.GetItemBySourceID(ctx, item.SourceID)
		if err := s.db.UpsertItem(ctx, &item); err != nil {
			return newCount, updatedCount, fmt.Errorf("upserting %s: %w", item.SourceID, err)
		}
		if existing == nil {
			newCount++
		} else {
			updatedCount++
		}
	}

	if err := s.refreshStaleItems(ctx, repo.Owner, repo.Name, fetchedSourceIDs); err != nil {
		return newCount, updatedCount, fmt.Errorf("refreshing stale items: %w", err)
	}

	return newCount, updatedCount, nil
}

func (s *Syncer) refreshStaleItems(ctx context.Context, owner, repo string, fetchedSourceIDs map[string]bool) error {
	openItems, err := s.db.ListItemsByRepoAndSourceState(ctx, owner, repo, "open")
	if err != nil {
		return err
	}

	for _, item := range openItems {
		if fetchedSourceIDs[item.SourceID] {
			continue
		}
		if item.Number == nil {
			continue
		}

		var (
			state    string
			fetchErr error
		)
		switch item.Kind {
		case "issue":
			state, fetchErr = s.ghClient.FetchIssueState(ctx, owner, repo, *item.Number)
		case "pr":
			state, fetchErr = s.ghClient.FetchPRState(ctx, owner, repo, *item.Number)
		default:
			continue
		}
		if fetchErr != nil {
			continue
		}

		if state != "open" {
			if err := s.db.UpdateItemSourceState(ctx, item.ID, state); err != nil {
				return fmt.Errorf("updating source state for %s: %w", item.SourceID, err)
			}
		}
	}

	return nil
}

// SourceIDForGitHub constructs the natural dedup key for a GitHub work item.
func SourceIDForGitHub(owner, repo, kind string, number int) string {
	return fmt.Sprintf("github:%s/%s:%s:%d", owner, repo, kind, number)
}

func workItemToInboxItem(wi ghclient.WorkItem) store.InboxItem {
	labelsJSON, _ := json.Marshal(wi.Labels)
	if wi.Labels == nil {
		labelsJSON = []byte("[]")
	}

	metadata := map[string]string{}
	if wi.HeadRef != "" {
		metadata["head_ref"] = wi.HeadRef
	}
	metadataJSON, _ := json.Marshal(metadata)

	number := wi.Number
	now := time.Now().UTC()
	updatedAt := wi.UpdatedAt

	sourceState := wi.State
	if sourceState == "" {
		sourceState = "open"
	}

	return store.InboxItem{
		SourceType:      "github",
		SourceID:        SourceIDForGitHub(wi.Owner, wi.Repo, string(wi.Kind), wi.Number),
		Kind:            string(wi.Kind),
		Number:          &number,
		Title:           wi.Title,
		Body:            wi.Body,
		Author:          wi.Author,
		Labels:          string(labelsJSON),
		Owner:           wi.Owner,
		Repo:            wi.Repo,
		Metadata:        string(metadataJSON),
		SourceState:     sourceState,
		Status:          store.ItemStatusNew,
		SourceUpdatedAt: &updatedAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
