package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// SetBaseURL sets the base URL for the GitHub API
func (c *Client) SetBaseURL(url string) {
	c.baseURL = url
}

// get makes a GET request to the GitHub API
func (c *Client) get(ctx context.Context, url string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit
	if resp.StatusCode == http.StatusForbidden {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    "API rate limit exceeded",
		}
	}

	// Check for other errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
		}
	}

	// Decode the response
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}


// GetRepository gets a repository from GitHub
func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)

	var result models.Repository
	err := c.get(ctx, url, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}


// func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error) {
//     url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)

//     req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
//     if err != nil {
//         return nil, fmt.Errorf("failed to create request: %w", err)
//     }

//     req.Header.Set("Authorization", "token "+c.token)
//     req.Header.Set("Accept", "application/vnd.github.v3+json")

//     resp, err := c.httpClient.Do(req)
//     if err != nil {
//         return nil, fmt.Errorf("failed to execute request: %w", err)
//     }
//     defer resp.Body.Close()

//     if resp.StatusCode != http.StatusOK {
//         return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
//     }

// 	fmt.Println(&resp.Body)
// 	var reponse models.Repository
//     if err := json.NewDecoder(resp.Body).Decode(&reponse); err != nil {
//         return nil, fmt.Errorf("failed to decode response: %w", err)
//     }


//     return &reponse, nil
// }

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

// APIError represents an error from the GitHub API
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API error: %d %s", e.StatusCode, e.Message)
}
