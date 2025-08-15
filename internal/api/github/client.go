package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	token      string
	httpClient *http.Client
}

type PullRequest struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	State       string     `json:"state"`
	User        User       `json:"user"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	MergedAt    *time.Time `json:"merged_at"`
	Draft       bool       `json:"draft"`
	Assignees   []User     `json:"assignees"`
	Labels      []Label    `json:"labels"`
	Additions   int        `json:"additions"`
	Deletions   int        `json:"deletions"`
	ChangedFiles int       `json:"changed_files"`
	Comments    int        `json:"comments"`
	ReviewComments int    `json:"review_comments"`
	HTMLURL     string     `json:"html_url"`
	Head        GitRef     `json:"head"`
	Base        GitRef     `json:"base"`
}

type User struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type GitRef struct {
	Ref  string     `json:"ref"`
	SHA  string     `json:"sha"`
	Repo Repository `json:"repo"`
}

type Repository struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    User   `json:"owner"`
}

type File struct {
	SHA         string `json:"sha"`
	Filename    string `json:"filename"`
	Status      string `json:"status"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Changes     int    `json:"changes"`
	Patch       string `json:"patch,omitempty"`
	ContentsURL string `json:"contents_url"`
}

type Comment struct {
	ID        int       `json:"id"`
	Body      string    `json:"body"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Review struct {
	ID          int       `json:"id"`
	User        User      `json:"user"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
}

type SearchResult struct {
	TotalCount int           `json:"total_count"`
	Items      []PullRequest `json:"items"`
}

func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) doRequest(method, path string, params url.Values) ([]byte, error) {
	u, err := url.Parse("https://api.github.com" + path)
	if err != nil {
		return nil, err
	}

	if params != nil {
		u.RawQuery = params.Encode()
	}

	req, err := http.NewRequest(method, u.String(), nil) //nolint:noctx
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) SearchPullRequests(query string) (*SearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("sort", "updated")
	params.Set("order", "desc")
	params.Set("per_page", "30")

	body, err := c.doRequest("GET", "/search/issues", params)
	if err != nil {
		return nil, err
	}

	var result SearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) GetPullRequest(owner, repo string, number int) (*PullRequest, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number)
	
	body, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var pr PullRequest
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, err
	}

	return &pr, nil
}

func (c *Client) GetPullRequestFiles(owner, repo string, number int) ([]File, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/files", owner, repo, number)
	params := url.Values{}
	params.Set("per_page", "100")
	
	body, err := c.doRequest("GET", path, params)
	if err != nil {
		return nil, err
	}

	var files []File
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, err
	}

	return files, nil
}

func (c *Client) GetPullRequestComments(owner, repo string, number int) ([]Comment, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, number)
	params := url.Values{}
	params.Set("per_page", "100")
	
	body, err := c.doRequest("GET", path, params)
	if err != nil {
		return nil, err
	}

	var comments []Comment
	if err := json.Unmarshal(body, &comments); err != nil {
		return nil, err
	}

	return comments, nil
}

func (c *Client) GetPullRequestReviews(owner, repo string, number int) ([]Review, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, number)
	params := url.Values{}
	params.Set("per_page", "100")
	
	body, err := c.doRequest("GET", path, params)
	if err != nil {
		return nil, err
	}

	var reviews []Review
	if err := json.Unmarshal(body, &reviews); err != nil {
		return nil, err
	}

	return reviews, nil
}
