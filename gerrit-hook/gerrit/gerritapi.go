package gerrit

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/zeebo/errs"
	"io/ioutil"
	"net/http"
	url2 "net/url"
)

var gerritBaseURL = "https://review.dev.storj.io"

type GerritClient struct {
	token string
}

func gerritHTTPCall(ctx context.Context, url string, result interface{}) error {

	httpRequest, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return errs.Wrap(err)
	}
	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return errs.Wrap(err)
	}

	if httpResponse.StatusCode >= 300 {
		return errs.New("couldn't get gerrit message from %s, code: %d", url, httpResponse.StatusCode)
	}
	body, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return errs.Wrap(err)
	}
	defer func() { _ = httpResponse.Body.Close() }()

	// XSSI prevention chars are removed here
	err = json.Unmarshal(body[5:], result)
	if err != nil {
		return errs.Wrap(err)
	}

	return nil
}

// GetCommitMessage retrieves the last long commit message of a gerrit patch.
func GetCommitMessage(ctx context.Context, changesetID string, commit string) (string, error) {
	c := Commit{}
	url := fmt.Sprintf("%s/changes/%s/revisions/%s/commit", gerritBaseURL, changesetID, commit)
	err := gerritHTTPCall(ctx, url, &c)
	if err != nil {
		return "", err
	}
	return c.Message, nil

}

func QueryChanges(ctx context.Context, condition string) (Changes, error) {
	c := Changes{}
	url := fmt.Sprintf("%s/changes/?q=%s", gerritBaseURL, url2.QueryEscape(condition))
	err := gerritHTTPCall(ctx, url, &c)
	return c, err
}

func QueryChange(ctx context.Context, change string) (Change, error) {
	c := Change{}
	url := fmt.Sprintf("%s/changes/%s/?o=labels", gerritBaseURL, change)
	err := gerritHTTPCall(ctx, url, &c)
	return c, err
}

type Commit struct {
	Message string `json:"message"`
}

type Changes []Change

type Change struct {
	Number           int           `json:"_number"`
	AttentionSet     struct{}      `json:"attention_set"`
	Branch           string        `json:"branch"`
	ChangeID         string        `json:"change_id"`
	Created          string        `json:"created"`
	Deletions        int           `json:"deletions"`
	HasReviewStarted bool          `json:"has_review_started"`
	Hashtags         []interface{} `json:"hashtags"`
	ID               string        `json:"id"`
	Insertions       int           `json:"insertions"`
	Mergeable        bool          `json:"mergeable"`
	MetaRevID        string        `json:"meta_rev_id"`
	Owner            struct {
		AccountID int `json:"_account_id"`
	} `json:"owner"`
	Project       string        `json:"project"`
	Requirements  []interface{} `json:"requirements"`
	Status        string        `json:"status"`
	Subject       string        `json:"subject"`
	SubmitRecords []struct {
		Labels []struct {
			AppliedBy struct {
				AccountID int `json:"_account_id"`
			} `json:"applied_by,omitempty"`
			Label  string `json:"label"`
			Status string `json:"status"`
		} `json:"labels"`
		RuleName string `json:"rule_name"`
		Status   string `json:"status"`
	} `json:"submit_records"`
	SubmitType             string `json:"submit_type"`
	TotalCommentCount      int    `json:"total_comment_count"`
	UnresolvedCommentCount int    `json:"unresolved_comment_count"`
	Updated                string `json:"updated"`
	Labels                 map[string]struct {
		All []struct {
			AccountId string `json:"account_id"`
			Value     int    `json:"value"`
		} `json:"all"`
	} `json:"labels"`
}

func (c Change) LabelMax(name string) int {
	res := 0
	for label, values := range c.Labels {
		if label == name {
			for _, vote := range values.All {
				if vote.Value > res {
					res = vote.Value
				}
			}
		}
	}
	return res
}

func (c Change) LabelMin(name string) int {
	res := 0
	for label, values := range c.Labels {
		if label == name {
			for _, vote := range values.All {
				if vote.Value < res {
					res = vote.Value
				}
			}
		}
	}
	return res
}

func (c Change) LabelCount(name string, value int) int {
	res := 0
	for label, values := range c.Labels {
		if label == name {
			for _, vote := range values.All {
				if vote.Value == value {
					res++
				}
			}
		}
	}
	return res
}
