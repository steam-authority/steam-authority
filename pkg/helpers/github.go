package helpers

import (
	"context"
	"sync"

	"github.com/gamedb/gamedb/pkg/config"
	"github.com/google/go-github/v27/github"
	"golang.org/x/oauth2"
)

var (
	githubContext = context.Background()
	githubClient  *github.Client
	githubMutex   sync.Mutex
)

func GetGithub() (*github.Client, context.Context) {

	githubMutex.Lock()
	defer githubMutex.Unlock()

	if githubClient == nil {
		githubClient = github.NewClient(oauth2.NewClient(
			githubContext,
			oauth2.StaticTokenSource(
				&oauth2.Token{
					AccessToken: config.Config.GithubToken.Get()},
			)))
	}

	return githubClient, githubContext
}
