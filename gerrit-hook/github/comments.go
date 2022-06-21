package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/storj/ci/gerrit-hook/gerrit"
	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type GithubClient struct {
	token string
	log   *zap.Logger
}

func NewClient(log *zap.Logger, token string) GithubClient {
	return GithubClient{
		token: token,
		log:   log,
	}
}

// AddComment handles incoming hook call by gerrit for patchset-created events.
func AddComment(ctx context.Context, project string, change string, commit string, changeUrl string, postComment func(ctc context.Context, orgRepo string, issue string, message string) error) error {

	message, err := gerrit.GetCommitMessage(ctx, change, commit)
	if err != nil {
		return err
	}
	previousMessage := ""

	currentRefs := findGithubRefs(message)
	oldRefs := findGithubRefs(previousMessage)
	newRefs := subtractRefs(currentRefs, oldRefs)

	for _, ref := range newRefs {
		if ref.repo == "" {
			ref.repo = project
		}
		comment := fmt.Sprintf("Change %s mentions this issue.", changeUrl)
		if err := postComment(ctx, ref.repo, ref.issue, comment); err != nil {
			return err
		}
	}
	return nil
}

type githubRef struct {
	repo  string
	issue string
}

// findGithubRefs tries to find references to a github issues / pull request.
func findGithubRefs(message string) (refs []githubRef) {
	issuePattern := regexp.MustCompile(`([a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+)?#(\d+)`)
	urlPattern := regexp.MustCompile(`https://github.com/([a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+)/(?:pull|issues)/(\d+)`)
	for _, line := range strings.Split(message, "\n") {
		matches := issuePattern.FindStringSubmatch(line)
		if matches != nil {
			refs = append(refs, githubRef{repo: matches[1], issue: matches[2]})
		}
		matches = urlPattern.FindStringSubmatch(line)
		if matches != nil {
			refs = append(refs, githubRef{repo: matches[1], issue: matches[2]})
		}
	}
	return refs
}

func subtractRefs(currentRefs, oldRefs []githubRef) []githubRef {
	newRefs := []githubRef{}
nextRef:
	for _, current := range currentRefs {
		for _, old := range oldRefs {
			if current == old {
				continue nextRef
			}
		}
		newRefs = append(newRefs, current)
	}
	return newRefs
}

// callGithubAPIV3 is a wrapper around the HTTP method call.
func (g *GithubClient) callGithubAPIV3(ctx context.Context, method string, url string, body io.Reader) error {
	client := &http.Client{}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return errs.Wrap(err)
	}

	req.Header.Add("Authorization", "token "+g.token)
	req.Header.Add("Accept", "application/vnd.github.v3+json")
	resp, err := client.Do(req)
	if err != nil {
		return errs.Wrap(err)
	}

	if resp.StatusCode > 299 {
		return errs.Combine(errs.New("%s url is failed (%s): %s", method, resp.Status, url), resp.Body.Close())
	}
	return resp.Body.Close()
}

// PostGithubComment adds a new comment to a github issue.
func (g *GithubClient) PostGithubComment(ctx context.Context, orgRepo string, issue string, message string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/issues/%s/comments", orgRepo, issue)
	request := map[string]string{
		"body": message,
	}
	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return errs.Wrap(err)
	}
	err = g.callGithubAPIV3(ctx, "POST", url, bytes.NewBuffer(jsonRequest))
	if err != nil {
		return err
	}
	return nil
}
