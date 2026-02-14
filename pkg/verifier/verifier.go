package verifier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// CronJob represents a cron job returned by the OpenClaw API
type CronJob struct {
	ID        string `json:"id"`
	Schedule  string `json:"schedule"`
	Command   string `json:"command"`
	CreatedAt int64  `json:"created_at"`
}

// CronAPIResponse is the response from OpenClaw's cron API
type CronAPIResponse struct {
	Crons []CronJob `json:"crons"`
}

// Checker is the interface for individual mechanism checkers
type Checker interface {
	Check(detectedAt time.Time) ([]string, error)
	Name() string
}

// VerificationResult contains the aggregated results of mechanism verification
type VerificationResult struct {
	IsBacked       bool
	Mechanisms     []string
	CheckedSources []string
}

// CronChecker queries the OpenClaw cron API for recently created jobs
type CronChecker struct {
	apiURL string
	client *http.Client
}

// NewCronChecker creates a checker that queries OpenClaw cron API
func NewCronChecker(apiURL string) *CronChecker {
	return &CronChecker{
		apiURL: apiURL,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Name returns the checker name for reporting
func (c *CronChecker) Name() string {
	return "crons"
}

// SetTimeout sets the HTTP client timeout
func (c *CronChecker) SetTimeout(d time.Duration) {
	c.client.Timeout = d
}

// Check queries the cron API for jobs created since detectedAt
func (c *CronChecker) Check(detectedAt time.Time) ([]string, error) {
	url := fmt.Sprintf("%s/api/v1/crons?since=%d", c.apiURL, detectedAt.Unix())

	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("cron API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cron API returned status %d", resp.StatusCode)
	}

	var apiResp CronAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("cron API response parse failed: %w", err)
	}

	var mechanisms []string
	for _, cron := range apiResp.Crons {
		if cron.CreatedAt < detectedAt.Unix() {
			continue
		}
		mechanisms = append(mechanisms, fmt.Sprintf("cron:%s", cron.ID))
	}
	return mechanisms, nil
}

// Verifier aggregates results from all mechanism checkers
type Verifier struct {
	checkers []Checker
}

// NewVerifier creates a verifier with the default set of checkers
func NewVerifier(cronAPIURL string) *Verifier {
	cronChecker := NewCronChecker(cronAPIURL)
	return &Verifier{
		checkers: []Checker{cronChecker},
	}
}

// SetTimeout sets the timeout for all checkers that support it
func (v *Verifier) SetTimeout(d time.Duration) {
	for _, checker := range v.checkers {
		if cc, ok := checker.(*CronChecker); ok {
			cc.SetTimeout(d)
		}
	}
}

// Verify runs all checkers and aggregates results.
// Individual checker failures are tolerated (graceful degradation).
func (v *Verifier) Verify(detectedAt time.Time) (*VerificationResult, error) {
	result := &VerificationResult{
		Mechanisms:     []string{},
		CheckedSources: []string{},
	}

	for _, checker := range v.checkers {
		result.CheckedSources = append(result.CheckedSources, checker.Name())

		mechanisms, err := checker.Check(detectedAt)
		if err != nil {
			// Graceful degradation: log but don't fail
			continue
		}
		result.Mechanisms = append(result.Mechanisms, mechanisms...)
	}

	result.IsBacked = len(result.Mechanisms) > 0
	return result, nil
}
