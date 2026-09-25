// Package ghissue registers maintainer-opened GitHub issues as workflow cases.
// The package performs no GitHub writes: its client exposes GET operations only.
package ghissue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ReasonUnparseable     = "unparseable-issue"
	ReasonNotOpen         = "not-open"
	ReasonNotMaintainer   = "not-maintainer"
	ReasonAlreadyAssigned = "already-assigned"
	ReasonHeldByLabel     = "held-by-label"
	ReasonCaseNotActive   = "case-not-active"
	ReasonEnsureFailed    = "ensure-failed"

	TriageBug          = "bug"
	TriageFeature      = "feature"
	TriageUnclassified = "unclassified"
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Issue is one parsed GitHub issue revision.
type Issue struct {
	Repository    string
	Number        int64
	Title         string
	Body          string
	State         string
	Author        string
	Assignees     []string
	Labels        []string
	URL           string
	UpdatedAt     time.Time
	Triage        string
	IsPullRequest bool
}

// ObjectID qualifies the issue with its repository: plain issue#n collides
// across repositories under the unique (mission, source, object, revision) key.
func (i Issue) ObjectID() string {
	return "github:" + i.Repository + "#" + strconv.FormatInt(i.Number, 10)
}

// RevisionID is the canonical UTC revision string.
func (i Issue) RevisionID() string {
	return i.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

// Unparseable is one wire item missing required fields.
type Unparseable struct {
	Raw any
}

// IssueLister is the read-only source consumed by the observer.
type IssueLister interface {
	ListIssues(ctx context.Context, cursor string) ([]any, []Unparseable, string, error)
	Name() string
}

func classifyWireItem(item any) error {
	object, ok := item.(map[string]any)
	if !ok {
		return errors.New("issue item is not an object")
	}
	if _, err := numberOf(object); err != nil {
		return err
	}
	for _, key := range []string{"title", "state", "updated_at", "html_url"} {
		if _, err := requiredString(object, key); err != nil {
			return err
		}
	}
	author, ok := object["user"].(map[string]any)
	if !ok {
		return errors.New("issue item has no user object")
	}
	if _, err := requiredString(author, "login"); err != nil {
		return err
	}
	if raw, present := object["body"]; present && raw != nil {
		if _, err := stringField(object, "body"); err != nil {
			return err
		}
	}
	for _, list := range []struct{ key, field string }{{"assignees", "login"}, {"labels", "name"}} {
		if _, err := nameList(object[list.key], list.field); err != nil {
			return err
		}
	}
	updated, err := requiredString(object, "updated_at")
	if err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, updated); err != nil {
		return fmt.Errorf("parse issue updated_at: %w", err)
	}
	return nil
}

func ParseIssue(item any, repository string) (Issue, error) {
	if err := classifyWireItem(item); err != nil {
		return Issue{}, err
	}
	object := item.(map[string]any)
	number, _ := numberOf(object)
	title, _ := requiredString(object, "title")
	state, _ := requiredString(object, "state")
	url, _ := requiredString(object, "html_url")
	updated, _ := requiredString(object, "updated_at")
	author, _ := requiredString(object["user"].(map[string]any), "login")
	body := ""
	if raw, ok := object["body"]; ok && raw != nil {
		value, err := stringField(object, "body")
		if err != nil {
			return Issue{}, err
		}
		body = value
	}
	assignees, err := nameList(object["assignees"], "login")
	if err != nil {
		return Issue{}, err
	}
	labels, err := nameList(object["labels"], "name")
	if err != nil {
		return Issue{}, err
	}
	updatedAt, err := time.Parse(time.RFC3339, updated)
	if err != nil {
		return Issue{}, fmt.Errorf("parse issue updated_at: %w", err)
	}
	issue := Issue{
		Repository: repository,
		Number:     number,
		Title:      title,
		Body:       body,
		State:      state,
		Author:     author,
		Assignees:  assignees,
		Labels:     labels,
		URL:        url,
		UpdatedAt:  updatedAt.UTC(),
	}
	issue.Triage = ClassifyTriage(labels, title)
	_, issue.IsPullRequest = object["pull_request"]
	return issue, nil
}

func ClassifyTriage(labels []string, title string) string {
	prefix := strings.ToLower(strings.TrimSpace(title))
	switch {
	case hasLabel(labels, "bug", "defect") || strings.HasPrefix(prefix, "[bug]"):
		return TriageBug
	case hasLabel(labels, "enhancement", "feature") ||
		strings.HasPrefix(prefix, "[feat]") || strings.HasPrefix(prefix, "[feature]"):
		return TriageFeature
	default:
		return TriageUnclassified
	}
}

func FilterIssues(issues []Issue, maintainers []string) ([]Issue, []ExcludedIssue, int) {
	allowed := make(map[string]bool, len(maintainers))
	for _, login := range maintainers {
		if trimmed := strings.ToLower(strings.TrimSpace(login)); trimmed != "" {
			allowed[trimmed] = true
		}
	}
	var kept []Issue
	var excluded []ExcludedIssue
	skipped := 0
	for _, issue := range issues {
		switch {
		case issue.IsPullRequest:
			skipped++
		case !strings.EqualFold(strings.TrimSpace(issue.State), "open"):
			excluded = append(excluded, ExcludedIssue{Issue: issue, Reason: ReasonNotOpen})
		case !allowed[strings.ToLower(strings.TrimSpace(issue.Author))]:
			excluded = append(excluded, ExcludedIssue{Issue: issue, Reason: ReasonNotMaintainer})
		case len(issue.Assignees) > 0:
			excluded = append(excluded, ExcludedIssue{Issue: issue, Reason: ReasonAlreadyAssigned})
		case hasLabel(issue.Labels, "box-hold", "wontfix"):
			excluded = append(excluded, ExcludedIssue{Issue: issue, Reason: ReasonHeldByLabel})
		default:
			kept = append(kept, issue)
		}
	}
	return kept, excluded, skipped
}

func CollectIssues(ctx context.Context, lister IssueLister) ([]Issue, []Unparseable, error) {
	if ctx == nil {
		return nil, nil, errors.New("collector context is required")
	}
	if lister == nil {
		return nil, nil, errors.New("issue lister is required")
	}
	repository := strings.TrimSpace(lister.Name())
	if !repositoryPattern.MatchString(repository) {
		return nil, nil, errors.New("issue lister name must be owner/name")
	}
	var (
		issues      []Issue
		unparseable []Unparseable
		seen        = map[string]struct{}{}
		cursor      string
	)
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		items, bad, next, err := lister.ListIssues(ctx, cursor)
		if err != nil {
			return nil, nil, err
		}
		unparseable = append(unparseable, bad...)
		for _, item := range items {
			issue, err := ParseIssue(item, repository)
			if err != nil {
				return nil, nil, fmt.Errorf("parse issue item: %w", err)
			}
			issues = append(issues, issue)
		}
		if next == "" {
			return issues, unparseable, nil
		}
		if _, repeated := seen[next]; repeated {
			return nil, nil, errors.New("GitHub issues pagination repeated a next cursor")
		}
		seen[next] = struct{}{}
		cursor = next
	}
}

func CanonicalSnapshot(issue Issue) ([]byte, error) {
	return json.Marshal(struct {
		Repo      string   `json:"repo"`
		Issue     int64    `json:"issue"`
		Title     string   `json:"title"`
		Body      string   `json:"body"`
		Author    string   `json:"author"`
		Assignees []string `json:"assignees"`
		Labels    []string `json:"labels"`
		URL       string   `json:"url"`
		UpdatedAt string   `json:"updatedAt"`
		Triage    string   `json:"triage"`
	}{
		Repo:      issue.Repository,
		Issue:     issue.Number,
		Title:     issue.Title,
		Body:      issue.Body,
		Author:    issue.Author,
		Assignees: nonNil(issue.Assignees),
		Labels:    nonNil(issue.Labels),
		URL:       issue.URL,
		UpdatedAt: issue.RevisionID(),
		Triage:    issue.Triage,
	})
}

func numberOf(object map[string]any) (int64, error) {
	var number int64
	switch value := object["number"].(type) {
	case float64:
		number = int64(value)
		if float64(number) != value {
			return 0, errors.New("issue number is not an integer")
		}
	case int64:
		number = value
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, errors.New("issue number is not an integer")
		}
		number = parsed
	default:
		return 0, errors.New("issue number is not numeric")
	}
	if number <= 0 {
		return 0, errors.New("issue number is not positive")
	}
	return number, nil
}

// requiredString reads a field that must be present and non-blank.
func requiredString(object map[string]any, key string) (string, error) {
	value, ok := object[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("issue %q is not a non-blank string", key)
	}
	return value, nil
}

// stringField reads a field that must be a string but may be blank; blank
// values are dropped by normalizeTokens.
func stringField(object map[string]any, key string) (string, error) {
	value, ok := object[key].(string)
	if !ok {
		return "", fmt.Errorf("issue %q is not a string", key)
	}
	return value, nil
}

func nameList(value any, key string) ([]string, error) {
	if value == nil {
		return []string{}, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("issue %s list is not an array", key)
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("issue %s list entry is not an object", key)
		}
		name, err := stringField(object, key)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return normalizeTokens(names), nil
}

func normalizeTokens(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func hasLabel(labels []string, wanted ...string) bool {
	for _, label := range labels {
		normalized := strings.ToLower(strings.TrimSpace(label))
		for _, want := range wanted {
			if normalized == want {
				return true
			}
		}
	}
	return false
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
