package providers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/caarlos0/log"

	"github.com/marcosnils/bin/pkg/assets"
)

type generic struct {
	url        string
	versionURL *url.URL
	client     *http.Client
}

func (g *generic) Fetch(opts *FetchOpts) (*File, error) {
	// Get version
	version, versionURL, err := g.GetLatestVersion()
	if err != nil {
		return nil, err
	}

	f := assets.NewFilter(&assets.FilterOpts{SkipScoring: opts.All, PackagePath: opts.PackagePath, SkipPathCheck: opts.SkipPatchCheck, PackageName: opts.PackageName})

	gf := &assets.FilteredAsset{URL: versionURL}

	outFile, err := f.ProcessURL(gf)
	if err != nil {
		return nil, err
	}

	// Set default name to last url path element if none was provided by the filter
	if outFile.Name == "" {
		outFile.Name = filepath.Base(gf.URL)
	}

	file := &File{Data: outFile.Source, Name: outFile.Name, Version: version, PackagePath: outFile.PackagePath}

	return file, nil
}

// GetLatestVersion checks the version url and
// returns the corresponding name and url to fetch the version
func (g *generic) GetLatestVersion() (string, string, error) {
	log.Debugf("Getting version from %s", g.versionURL.String())

	resp, err := g.client.Get(g.versionURL.String())
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	version := strings.TrimSpace(string(content))

	versionURLString := strings.ReplaceAll(g.url, "{version}", version)

	versionURL, err := url.Parse(versionURLString)
	if err != nil {
		return "", "", err
	}

	return version, versionURL.String(), nil
}

func (g *generic) GetID() string {
	return "generic"
}

func newGeneric(u, versionURL string) (Provider, error) {
	// Validate the versionURL
	lurl, err := url.Parse(versionURL)
	if err != nil {
		return nil, fmt.Errorf("invalid versionURL: %w", err)
	}

	return &generic{url: u, versionURL: lurl, client: http.DefaultClient}, nil
}
