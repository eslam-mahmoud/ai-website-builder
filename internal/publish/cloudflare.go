package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// cloudflareClient polls Cloudflare Pages for deployment status. With the
// recommended GitHub-connected Pages setup (spec §9.8 Option A), Cloudflare
// starts a deployment automatically when the repo is pushed; we watch for
// the deployment matching our commit and report its outcome.
type cloudflareClient struct {
	apiKey    string
	accountID string
	http      *http.Client
}

func newCloudflareClient(apiKey, accountID string) *cloudflareClient {
	return &cloudflareClient{apiKey: apiKey, accountID: accountID,
		http: &http.Client{Timeout: 30 * time.Second}}
}

type cfDeployment struct {
	ID                string `json:"id"`
	DeploymentTrigger struct {
		Metadata struct {
			CommitHash string `json:"commit_hash"`
		} `json:"metadata"`
	} `json:"deployment_trigger"`
	LatestStage struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"latest_stage"`
}

func (c *cloudflareClient) listDeployments(ctx context.Context, project string) ([]cfDeployment, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/pages/projects/%s/deployments",
		c.accountID, project)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Success bool           `json:"success"`
		Result  []cfDeployment `json:"result"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode cloudflare response: %w", err)
	}
	if !body.Success {
		msg := resp.Status
		if len(body.Errors) > 0 {
			msg = body.Errors[0].Message
		}
		return nil, fmt.Errorf("cloudflare api: %s", msg)
	}
	return body.Result, nil
}

// AwaitDeployment waits for the Pages deployment triggered by commitHash and
// returns its ID once it succeeds. It returns an error if the deployment
// fails; if nothing matching appears before timeout it returns "" with no
// error (the push itself already succeeded).
func (c *cloudflareClient) AwaitDeployment(ctx context.Context, project, commitHash string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		deps, err := c.listDeployments(ctx, project)
		if err != nil {
			return "", err
		}
		for _, d := range deps {
			if d.DeploymentTrigger.Metadata.CommitHash != commitHash {
				continue
			}
			switch d.LatestStage.Status {
			case "failure", "failed", "canceled":
				return d.ID, fmt.Errorf("cloudflare pages deployment failed at stage %q", d.LatestStage.Name)
			case "success":
				if d.LatestStage.Name == "deploy" {
					return d.ID, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return "", nil
}
