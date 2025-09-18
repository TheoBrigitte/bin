package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/caarlos0/log"
	"github.com/google/go-github/v31/github"
	"golang.org/x/oauth2"

	"github.com/marcosnils/bin/pkg/assets"
)

type gitHub struct {
	url    *url.URL
	client *github.Client
	owner  string
	repo   string
	tag    string
	asset  string
	token  string
}

func (g *gitHub) Fetch(opts *FetchOpts) (*File, error) {
	var release *github.RepositoryRelease

	// If we have a tag, let's fetch from there
	var err error
	var resp *github.Response
	if len(g.tag) > 0 || len(opts.Version) > 0 {
		if len(opts.Version) > 0 {
			// this is used by for the `ensure` command
			g.tag = opts.Version
		}
		log.Infof("Getting %s release for %s/%s", g.tag, g.owner, g.repo)
		release, _, err = g.client.Repositories.GetReleaseByTag(context.TODO(), g.owner, g.repo, g.tag)
	} else {
		log.Infof("Getting latest release for %s/%s", g.owner, g.repo)
		release, resp, err = g.client.Repositories.GetLatestRelease(context.TODO(), g.owner, g.repo)
		if resp.StatusCode == http.StatusNotFound {
			err = fmt.Errorf("repository %s/%s does not have releases", g.owner, g.repo)
		}
	}

	if err != nil {
		return nil, err
	}

	candidates := getCandidates(release.Assets, g.asset)
	f := assets.NewFilter(&assets.FilterOpts{SkipScoring: opts.All, PackagePath: opts.PackagePath, SkipPathCheck: opts.SkipPatchCheck, PackageName: opts.PackageName})

	gf, err := f.FilterAssets(g.repo, candidates)
	if err != nil {
		return nil, err
	}

	gf.ExtraHeaders = map[string]string{"Accept": "application/octet-stream"}
	if g.token != "" {
		gf.ExtraHeaders["Authorization"] = fmt.Sprintf("token %s", g.token)
	}

	outFile, err := f.ProcessURL(gf)
	if err != nil {
		return nil, err
	}

	version := release.GetTagName()

	// TODO calculate file hash. Not sure if we can / should do it here
	// since we don't want to read the file unnecesarily. Additionally, sometimes
	// releases have .sha256 files, so it'd be nice to check for those also
	file := &File{Data: outFile.Source, Name: outFile.Name, Version: version, PackagePath: outFile.PackagePath}

	return file, nil
}

// getCandidates returns a list of assets to be used as candidates for filtering
// If userAsset is provided, it will try to find that asset and return it as the only candidate
func getCandidates(githubAssets []*github.ReleaseAsset, userAsset string) []*assets.Asset {
	candidates := []*assets.Asset{}
	foundUserAsset := false
	for _, a := range githubAssets {
		if userAsset != "" && a.GetName() == userAsset {
			foundUserAsset = true
			candidates = []*assets.Asset{&assets.Asset{Name: a.GetName(), URL: a.GetURL()}}
			break
		}
		candidates = append(candidates, &assets.Asset{Name: a.GetName(), URL: a.GetURL()})
	}

	if userAsset != "" && !foundUserAsset {
		log.Warnf("asset %s not found in release", userAsset)
	}

	return candidates
}

// GetLatestVersion checks the latest repo release and
// returns the corresponding name and url to fetch the version
func (g *gitHub) GetLatestVersion() (string, string, error) {
	log.Debugf("Getting latest release for %s/%s", g.owner, g.repo)
	release, _, err := g.client.Repositories.GetLatestRelease(context.TODO(), g.owner, g.repo)
	if err != nil {
		return "", "", err
	}

	return release.GetTagName(), release.GetHTMLURL(), nil
}

func (g *gitHub) GetID() string {
	return "github"
}

func newGitHub(u *url.URL) (Provider, error) {
	// Supported Github URL formats:
	// - https://github.com/owner/repo
	// - https://github.com/owner/repo/releases/tag/v1.2.3
	// - https://github.com/owner/repo/releases/download/v1.2.3
	// - https://github.com/owner/repo/releases/download/v1.2.3/asset-name
	splitedPath := strings.Split(u.Path, "/")
	if len(splitedPath) < 3 {
		return nil, fmt.Errorf("error parsing Github URL %s, can't find owner and repo", u.String())
	}

	// Github repository owner and name are always the 2nd and 3rd path elements
	owner := splitedPath[1]
	repo := splitedPath[2]

	// If the URL is a release or download URL, try to get the tag and asset name
	// otherwise, latest release will be used
	var tag string
	var asset string
	if len(splitedPath) > 5 && splitedPath[3] == "releases" {
		// In release and download URL's, the tag is the 6th element
		tag = splitedPath[5]
		if len(splitedPath) > 6 && splitedPath[4] == "download" {
			// In download URL's, the asset name is the 7th element
			asset = splitedPath[6]
		}
	}

	token := os.Getenv("GITHUB_AUTH_TOKEN")
	if len(token) == 0 {
		token = os.Getenv("GITHUB_TOKEN")
	}

	// GHES client
	gbu := os.Getenv("GHES_BASE_URL")
	guu := os.Getenv("GHES_UPLOAD_URL")
	gau := os.Getenv("GHES_AUTH_TOKEN")

	var tc *http.Client

	if len(gbu) > 0 && len(guu) > 0 && len(gau) > 0 {
		tc = oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: gau},
		))
	} else if token != "" {
		tc = oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		))
	}

	var client *github.Client
	var err error

	if len(gbu) > 0 && len(guu) > 0 && len(gau) > 0 {
		if client, err = github.NewEnterpriseClient(gbu, guu, tc); err != nil {
			return nil, fmt.Errorf("error initializing GHES client %v", err)
		}
	} else {
		client = github.NewClient(tc)
	}

	return &gitHub{url: u, client: client, owner: owner, repo: repo, tag: tag, asset: asset, token: token}, nil
}
