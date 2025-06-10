package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github-service/internal/config"
	"github-service/internal/models"
)

type GitHubClient interface {
    GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error)
    GetCommits(ctx context.Context, owner, repo string, since *time.Time, page, perPage int) ([]*models.GitHubCommit, error)
	ParseRepositoryResponse(resp *http.Response) (*models.Repository, error)
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

func NewClient(token string) *Client {
	cfg, _ := config.Load()
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token:   token,
		baseURL: cfg.GitHubBaseUrl,
	}
}

func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error) {
    url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Authorization", "token "+c.token)
    req.Header.Set("Accept", "application/vnd.github.v3+json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to execute request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
    }

	var reponse models.Repository
    if err := json.NewDecoder(resp.Body).Decode(&reponse); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }

    return &reponse, nil
}

func (c *Client) ParseRepositoryResponse(resp *http.Response) (*models.Repository, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status)
	}

	var repo models.Repository
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return nil, fmt.Errorf("failed to decode repository response: %w", err)
	}

	return &repo, nil
}

func (c *Client) GetCommits(ctx context.Context, owner, repo string, since *time.Time, page, perPage int) ([]*models.GitHubCommit, error) {
	
	u, err := url.Parse(fmt.Sprintf("%s/repos/%s/%s/commits", c.baseURL, owner, repo))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	q.Set("since", since.Format(time.RFC3339))
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("per_page", fmt.Sprintf("%d", perPage))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var commits []*models.GitHubCommit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return commits, nil
}
