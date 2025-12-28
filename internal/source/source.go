package source

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"ghpm/internal/manifest"
)

type Release struct {
	Tag       string
	ID        int64
	Published time.Time
	Assets    []Asset
}

type Asset struct {
	Name string
	URL  string
	Size int64
}

type Resolver interface {
	ResolveRelease(repo string, version string) (Release, error)
}

func NewResolver(kind string, client *http.Client) (Resolver, error) {
	switch kind {
	case "github":
		return &githubResolver{client: client}, nil
	case "gitlab":
		return &gitlabResolver{client: client}, nil
	case "http":
		return &httpResolver{}, nil
	default:
		return nil, fmt.Errorf("unknown source kind %q", kind)
	}
}

type httpResolver struct{}

func (r *httpResolver) ResolveRelease(repo string, version string) (Release, error) {
	if version == "" {
		return Release{}, errors.New("http source requires explicit --version")
	}
	return Release{Tag: version}, nil
}

type githubResolver struct {
	client *http.Client
}

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	ID         int64         `json:"id"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Published  time.Time     `json:"published_at"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

func (r *githubResolver) ResolveRelease(repo string, version string) (Release, error) {
	releases, err := r.listReleases(repo)
	if err != nil {
		return Release{}, err
	}
	if len(releases) == 0 {
		return Release{}, fmt.Errorf("no releases found for %s", repo)
	}
	if version != "" {
		for _, rel := range releases {
			if rel.TagName == version {
				return mapGitHubRelease(rel), nil
			}
		}
		return Release{}, fmt.Errorf("version %s not found", version)
	}
	sort.Slice(releases, func(i, j int) bool {
		return compareReleases(releases[i].TagName, releases[j].TagName, releases[i].Published, releases[j].Published) > 0
	})
	return mapGitHubRelease(releases[0]), nil
}

func (r *githubResolver) listReleases(repo string) ([]githubRelease, error) {
	u := fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases: %s", resp.Status)
	}
	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	filtered := make([]githubRelease, 0, len(releases))
	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered, nil
}

func mapGitHubRelease(rel githubRelease) Release {
	assets := make([]Asset, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		assets = append(assets, Asset{
			Name: a.Name,
			URL:  a.URL,
			Size: a.Size,
		})
	}
	return Release{
		Tag:       rel.TagName,
		ID:        rel.ID,
		Published: rel.Published,
		Assets:    assets,
	}
}

type gitlabResolver struct {
	client *http.Client
}

type gitlabRelease struct {
	TagName  string `json:"tag_name"`
	Released string `json:"released_at"`
	Assets   struct {
		Links []gitlabAsset `json:"links"`
	} `json:"assets"`
}

type gitlabAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (r *gitlabResolver) ResolveRelease(repo string, version string) (Release, error) {
	releases, err := r.listReleases(repo)
	if err != nil {
		return Release{}, err
	}
	if len(releases) == 0 {
		return Release{}, fmt.Errorf("no releases found for %s", repo)
	}
	if version != "" {
		for _, rel := range releases {
			if rel.TagName == version {
				return mapGitLabRelease(rel), nil
			}
		}
		return Release{}, fmt.Errorf("version %s not found", version)
	}
	sort.Slice(releases, func(i, j int) bool {
		return compareReleases(releases[i].TagName, releases[j].TagName, parseGitLabTime(releases[i].Released), parseGitLabTime(releases[j].Released)) > 0
	})
	return mapGitLabRelease(releases[0]), nil
}

func (r *gitlabResolver) listReleases(repo string) ([]gitlabRelease, error) {
	project := url.PathEscape(repo)
	u := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/releases", project)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab releases: %s", resp.Status)
	}
	var releases []gitlabRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func mapGitLabRelease(rel gitlabRelease) Release {
	assets := make([]Asset, 0, len(rel.Assets.Links))
	for _, a := range rel.Assets.Links {
		assets = append(assets, Asset{
			Name: a.Name,
			URL:  a.URL,
		})
	}
	return Release{
		Tag:    rel.TagName,
		Assets: assets,
	}
}

func compareReleases(tagA, tagB string, timeA, timeB time.Time) int {
	if score, ok := compareSemver(tagA, tagB); ok {
		return score
	}
	if timeA.After(timeB) {
		return 1
	}
	if timeB.After(timeA) {
		return -1
	}
	return strings.Compare(tagA, tagB)
}

func compareSemver(a, b string) (int, bool) {
	va, oka := parseSemver(a)
	vb, okb := parseSemver(b)
	if !oka || !okb {
		return 0, false
	}
	for i := 0; i < 3; i++ {
		if va[i] > vb[i] {
			return 1, true
		}
		if va[i] < vb[i] {
			return -1, true
		}
	}
	return 0, true
}

func parseSemver(tag string) ([3]int, bool) {
	tag = strings.TrimPrefix(tag, "v")
	parts := strings.Split(tag, ".")
	if len(parts) < 2 {
		return [3]int{}, false
	}
	var nums [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n := 0
		for _, ch := range parts[i] {
			if ch < '0' || ch > '9' {
				break
			}
			n = n*10 + int(ch-'0')
		}
		nums[i] = n
	}
	return nums, true
}

func parseGitLabTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	return time.Time{}
}

func SelectAsset(release Release, action manifest.AssetAction) (Asset, error) {
	if action.Name != "" {
		for _, asset := range release.Assets {
			if asset.Name == action.Name {
				return asset, nil
			}
		}
		return Asset{}, fmt.Errorf("asset %s not found", action.Name)
	}
	if action.Pattern != "" {
		for _, asset := range release.Assets {
			if manifest.MatchPattern(asset.Name, action.Pattern) {
				return asset, nil
			}
		}
		return Asset{}, fmt.Errorf("asset matching %q not found", action.Pattern)
	}
	return Asset{}, errors.New("asset action requires name or pattern")
}

func NormalizeRepoRepoName(repo string) string {
	return path.Base(repo)
}
