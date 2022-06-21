// Copyright (C) 2021 Storj Labs, Inc.
// See LICENSE for copying information.

package main

import (
	"context"
	"fmt"
	"github.com/spf13/viper"
	"github.com/storj/ci/gerrit-hook/github"
	"github.com/storj/ci/gerrit-hook/jenkins"
	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"strings"
	"time"
)

var gerritBaseURL = "https://review.dev.storj.io"

// main is a binary which can be copied to gerrit's hooks directory and can act based on the give parameters.
func main() {

	cfg := zap.NewDevelopmentConfig()

	// directory to collect events for debug
	logDir := "/tmp/gerrit-hook-log"
	if _, err := os.Stat(logDir); err == nil {
		cfg.OutputPaths = append(cfg.OutputPaths, path.Join(logDir, "hook.log"))
	}

	log, _ := cfg.Build()

	viper.SetConfigName("config")
	viper.AddConfigPath(path.Join(path.Base(os.Args[0])))
	viper.AddConfigPath("$HOME/.gerrit-hook")
	viper.AddConfigPath("$HOME/.config/gerrit-hook")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		log.Warn("Reading configration files are failed. Hope you use environment variables (JENKINS_USER, JENKINS_TOKEN, GITHUB_TOKEN)", zap.Error(err))
	}

	viper.SetConfigName("gerrit-hook")

	j := jenkins.NewClient(log.Named("jenkins"), viper.GetString("jenkins-user"), viper.GetString("jenkins-token"), viper.GetStringSlice("projects"))

	g := github.NewClient(log.Named("github"), viper.GetString("github-token"))

	// arguments are defined by gerrit hook system, usually (but not only) --key value about the build
	argMap := map[string]string{}
	for p := 1; p < len(os.Args); p++ {
		if len(os.Args) > p && !strings.HasPrefix(os.Args[p+1], "--") {
			argMap[os.Args[p][2:]] = os.Args[p+1]
			p++
		}
	}

	// directory to collect events for debug
	debugDir := "/tmp/gerrit-hook-debug"
	if _, err := os.Stat(debugDir); err == nil {
		filename := fmt.Sprintf("%s-%d.txt", time.Now().Format("20060102-150405"), rand.Int())
		err := ioutil.WriteFile(path.Join(debugDir, filename), []byte(strings.Join(os.Args, "\n")), 0644)
		if err != nil {
			log.Error("couldn't write out debug information", zap.Error(err))
		}
	}
	// binary is symlinked to site/hooks under the name of default hook name:
	// https://gerrit.googlesource.com/plugins/hooks/+/HEAD/src/main/resources/Documentation/config.md
	action := path.Base(os.Args[0])

	// helping local development
	if os.Getenv("GERRIT_HOOK_ARGFILE") != "" {
		argMap, action, err = readArgFile(os.Getenv("GERRIT_HOOK_ARGFILE"))
	}

	log.Debug("Hook is called",
		zap.String("action", action),
		zap.String("project", argMap["project"]),
		zap.String("change", argMap["change"]),
	)

	err = triggerAction(action, &j, &g, argMap)
	if err != nil {
		log.Error("Triggering action is failed", zap.Error(err))
	}
}

func readArgFile(fileName string) (argMap map[string]string, action string, err error) {
	content, err := ioutil.ReadFile(fileName)
	if err != nil {
		return argMap, action, errs.Wrap(err)
	}

	argMap = make(map[string]string)
	action = ""
	key := ""
	value := ""
	for _, line := range strings.Split(string(content), "\n") {
		if action == "" {
			action = path.Base(line)
			continue
		}
		if strings.HasPrefix(line, "--") {
			if key != "" {
				argMap[key] = value
			}
			key = line[2:]
			value = ""
		} else {
			if value != "" {
				value += "\n"
			}
			value += line
		}
	}
	if key != "" {
		argMap[key] = value
	}
	return argMap, action, nil
}

func triggerAction(action string, j *jenkins.Client, g *github.GithubClient, argMap map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	switch action {
	case "patchset-created":
		return errs.Combine(
			github.AddComment(ctx, argMap["project"], argMap["change"], argMap["commit"], argMap["change-url"], g.PostGithubComment),
			j.TriggeredByNewPatch(ctx, argMap["project"], argMap["change"], argMap["commit"]),
		)
	case "comment-added":
		return errs.Combine(
			j.TriggeredByComment(ctx, argMap["project"], argMap["change"], argMap["commit"], argMap["comment"]),
			j.TriggeredBySuccessVerify(ctx, argMap["project"], argMap["change"], argMap["commit"], argMap["comment"]),
		)
	}
	return nil
}
