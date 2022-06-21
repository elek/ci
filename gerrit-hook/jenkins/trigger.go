package jenkins

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/storj/ci/gerrit-hook/gerrit"
	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"strings"
)

var premergeCondition = func(change gerrit.Change) bool {
	if change.LabelMax("Verified") == 1 && change.LabelCount("Code-Review", 2) > 1 && change.LabelMin("Code-Review") > -2 {
		return true
	}
	return false
}

var verifyCondition = func(change gerrit.Change) bool {
	if change.LabelMax("Verified") == 0 {
		return true
	}
	return false
}

// Client contains all the information to call jenkins instances.
type Client struct {
	Username string
	Token    string
	log      *zap.Logger
	projects []string
}

// NewClient creates a new client instance.
func NewClient(log *zap.Logger, username string, token string, projects []string) Client {
	return Client{
		Username: username,
		Token:    token,
		log:      log,
		projects: projects,
	}
}

func (c *Client) TriggeredBySuccessVerify(ctx context.Context, project string, change string, commit string, comment string) error {
	if !c.enabledProject(project) {
		return nil
	}
	if strings.Contains(comment, "build verify is finished successfully") {

		return c.triggerJobIfRequired(ctx, change, commit, jenkinsProject(project)+"-gerrit-premerge", premergeCondition)
	}
	return nil
}

func (c *Client) TriggeredByComment(ctx context.Context, project string, change string, commit string, comment string) error {
	if !c.enabledProject(project) {
		return nil
	}

	buildType := ""
	if strings.Contains(comment, "run jenkins verify") {
		buildType = "verify"
	} else if strings.Contains(comment, "run jenkins premerge") {
		buildType = "premerge"
	} else if strings.Contains(comment, "run jenkins") {
		buildType = "verify"
	} else {
		return nil
	}

	c.log.Info("Triggering jenkins build is required", zap.String("project", project), zap.String("change", change), zap.String("comment", comment))

	return c.triggerJobIfRequired(ctx, change, commit, jenkinsProject(project)+"-gerrit-"+buildType, func(change gerrit.Change) bool {
		return true
	})

}

func (c *Client) TriggeredByNewPatch(ctx context.Context, project string, change string, commit string) error {
	if !c.enabledProject(project) {
		return nil
	}
	return errs.Combine(
		c.triggerJobIfRequired(ctx, change, commit, jenkinsProject(project)+"-gerrit-verify", verifyCondition),
		c.triggerJobIfRequired(ctx, change, commit, jenkinsProject(project)+"-gerrit-premerge", premergeCondition),
	)

}

func (c *Client) triggerJobIfRequired(ctx context.Context, changeId string, commit string, job string, condition func(gerrit.Change) bool) error {
	change, err := gerrit.QueryChange(ctx, changeId)
	if err != nil {
		return err
	}

	// not in the state what we are interested about
	if !condition(change) {
		return nil
	}

	// trigger jenkins job
	err = c.TriggerJob(ctx, job, map[string]string{"GERRIT_REF": commit})
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) TriggerJob(ctx context.Context, job string, parameters map[string]string) error {
	var params []string
	for k, v := range parameters {
		params = append(params, k+"="+v)
	}
	triggerUrl := fmt.Sprintf("https://build.dev.storj.io/job/%s/buildWithParameters?%s", job, strings.Join(params, "="))
	return c.jenkinsHTTPCall(ctx, triggerUrl, nil)
}

func (c *Client) jenkinsHTTPCall(ctx context.Context, url string, result interface{}) error {
	httpRequest, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return errs.Wrap(err)
	}
	httpRequest.SetBasicAuth(c.Username, c.Token)
	c.log.Info("Executing HTTP request", zap.String("url", url), zap.String("user", c.Username))

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

	if result != nil {
		err = json.Unmarshal(body, result)
		if err != nil {
			return errs.Wrap(err)
		}
	}

	return nil
}

func (c *Client) enabledProject(project string) bool {
	for _, p := range c.projects {
		if p == project {
			return true
		}
	}
	return false
}

func jenkinsProject(project string) string {
	parts := strings.Split(project, "/")
	return parts[len(parts)-1]
}
