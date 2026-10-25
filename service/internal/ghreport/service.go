// Package ghreport builds ordered reports from GitHub issue pages.
package ghreport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"time"

	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/cache"
	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/config"
	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/github"
	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/lib/logger/sl"
	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/report"
)

// Service runs the report workflow for the configured repositories.
type Service struct {
	Config config.Config
	Client *github.Client
	Log    *slog.Logger
	Now    func() time.Time
}

// Run processes repositories in input order and reports any fatal or partial failure.
func (s *Service) Run(ctx context.Context) (report.Report, github.ErrorCode) {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	result := report.Report{GeneratedAt: now().UTC().Format(time.RFC3339), Repos: make([]report.Repository, 0, len(s.Config.Repos))}
	failed := 0
	allFromCache := true
	for _, name := range s.Config.Repos {
		repo, code, fromCache := s.fetchRepo(ctx, name)
		allFromCache = allFromCache && fromCache
		if code == github.Unauthorized {
			return report.Report{}, code
		}
		if code != "" {
			failed++
			repo.Fail(string(code))
			if s.Log != nil {
				s.Log.Warn("repository failed", slog.String("repo", name), slog.String("code", string(code)))
			}
		}
		result.Repos = append(result.Repos, repo)
	}
	if failed == len(s.Config.Repos) {
		return report.Report{}, github.ErrorCode("all_failed")
	}
	if key, err := json.Marshal(struct {
		APIBase string   `json:"api_base"`
		Repos   []string `json:"repos"`
		Limit   int      `json:"limit"`
		Since   string   `json:"since"`
	}{s.Config.APIBase.String(), s.Config.Repos, s.Config.Limit, s.Config.Since.Format("2006-01-02")}); err == nil {
		result.GeneratedAt = (cache.Store{Dir: s.Config.CacheDir}).ReportTime(string(key), result.GeneratedAt, allFromCache)
	}
	if failed > 0 {
		return result, github.ErrorCode("partial_failure")
	}
	return result, ""
}

func (s *Service) fetchRepo(ctx context.Context, name string) (report.Repository, github.ErrorCode, bool) {
	repo := report.NewRepository(name)
	address := firstURL(s.Config.APIBase, name)
	seenPages := make(map[string]bool)
	seenIssues := make(map[int]bool)
	times := make(map[int]time.Time)
	allFromCache := true
	for address != "" {
		if seenPages[address] {
			return repo, github.InvalidResponse, false
		}
		seenPages[address] = true
		page, code := s.Client.Fetch(ctx, address)
		if code != "" {
			return repo, code, false
		}
		allFromCache = allFromCache && page.FromCache
		var items []json.RawMessage
		if len(page.Body) == 0 || page.Body[0] != '[' || json.Unmarshal(page.Body, &items) != nil {
			return repo, github.InvalidResponse, false
		}
		for _, raw := range items {
			issue, created, pullRequest, err := decodeIssue(raw)
			if err != nil {
				if s.Log != nil {
					s.Log.Debug("invalid GitHub issue", sl.Err(err))
				}
				return repo, github.InvalidResponse, false
			}
			if pullRequest || seenIssues[issue.Number] || (s.Config.HasSince && created.Before(s.Config.Since)) {
				continue
			}
			seenIssues[issue.Number] = true
			times[issue.Number] = created
			repo.Issues = append(repo.Issues, issue)
			if s.Config.Limit > 0 && len(repo.Issues) >= s.Config.Limit {
				break
			}
		}
		if s.Config.Limit > 0 && len(repo.Issues) >= s.Config.Limit {
			break
		}
		address = page.Next
	}
	sort.Slice(repo.Issues, func(i, j int) bool {
		left, right := repo.Issues[i], repo.Issues[j]
		if times[left.Number].Equal(times[right.Number]) {
			return left.Number < right.Number
		}
		return times[left.Number].Before(times[right.Number])
	})
	repo.Count = len(repo.Issues)
	return repo, "", allFromCache
}

func firstURL(base *url.URL, name string) string {
	parts := strings.Split(name, "/")
	copyURL := *base
	copyURL.Path = strings.TrimRight(base.Path, "/") + "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) + "/issues"
	query := copyURL.Query()
	query.Set("state", "open")
	query.Set("sort", "created")
	query.Set("direction", "asc")
	query.Set("per_page", "100")
	copyURL.RawQuery = query.Encode()
	return copyURL.String()
}

func decodeIssue(raw json.RawMessage) (report.Issue, time.Time, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return report.Issue{}, time.Time{}, false, errors.New("issue is not an object")
	}
	if _, exists := fields["pull_request"]; exists {
		return report.Issue{}, time.Time{}, true, nil
	}
	var parsed struct {
		Number *int    `json:"number"`
		Title  *string `json:"title"`
		User   *struct {
			Login *string `json:"login"`
		} `json:"user"`
		Labels    []json.RawMessage `json:"labels"`
		CreatedAt *string           `json:"created_at"`
		HTMLURL   *string           `json:"html_url"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Number == nil || parsed.Title == nil || parsed.User == nil || parsed.User.Login == nil || parsed.Labels == nil || parsed.CreatedAt == nil || parsed.HTMLURL == nil || *parsed.Number < 1 {
		return report.Issue{}, time.Time{}, false, errors.New("issue has missing or invalid fields")
	}
	created, err := time.Parse(time.RFC3339, *parsed.CreatedAt)
	if err != nil {
		return report.Issue{}, time.Time{}, false, fmt.Errorf("invalid created_at: %w", err)
	}
	labels := make([]string, 0, len(parsed.Labels))
	for _, rawLabel := range parsed.Labels {
		var label string
		if len(rawLabel) > 0 && rawLabel[0] == '"' {
			if err := json.Unmarshal(rawLabel, &label); err != nil {
				return report.Issue{}, time.Time{}, false, errors.New("invalid label")
			}
		} else {
			var object struct {
				Name *string `json:"name"`
			}
			if err := json.Unmarshal(rawLabel, &object); err != nil || object.Name == nil {
				return report.Issue{}, time.Time{}, false, errors.New("invalid label")
			}
			label = *object.Name
		}
		labels = append(labels, label)
	}
	return report.Issue{Number: *parsed.Number, Title: *parsed.Title, Author: *parsed.User.Login, Labels: labels, CreatedAt: *parsed.CreatedAt, URL: *parsed.HTMLURL}, created, false, nil
}
