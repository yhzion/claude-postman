package updater

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHTTPClient struct {
	responses map[string]*http.Response
}

func (m *mockHTTPClient) Get(url string) (*http.Response, error) {
	if resp, ok := m.responses[url]; ok {
		return resp, nil
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("not found")),
	}, nil
}

func newMockClient(body string) *mockHTTPClient {
	return &mockHTTPClient{
		responses: map[string]*http.Response{
			apiURL: {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
			},
		},
	}
}

const sampleRelease = `{
	"tag_name": "v0.2.0",
	"assets": [
		{"name": "claude-postman-linux-amd64", "browser_download_url": "https://example.com/linux-amd64"},
		{"name": "claude-postman-darwin-arm64", "browser_download_url": "https://example.com/darwin-arm64"}
	]
}`

func TestCheckLatest(t *testing.T) {
	u := &Updater{CurrentVersion: "v0.1.0", Client: newMockClient(sampleRelease)}
	rel, err := u.CheckLatest()
	require.NoError(t, err)
	assert.Equal(t, "v0.2.0", rel.TagName)
	assert.Len(t, rel.Assets, 2)
}

func TestIsNewer_Yes(t *testing.T) {
	u := &Updater{CurrentVersion: "v0.1.0"}
	rel := &Release{TagName: "v0.2.0"}
	assert.True(t, u.IsNewer(rel))
}

func TestIsNewer_Same(t *testing.T) {
	u := &Updater{CurrentVersion: "v0.2.0"}
	rel := &Release{TagName: "v0.2.0"}
	assert.False(t, u.IsNewer(rel))
}

func TestIsNewer_Dev(t *testing.T) {
	u := &Updater{CurrentVersion: "dev"}
	rel := &Release{TagName: "v0.2.0"}
	assert.False(t, u.IsNewer(rel), "dev builds should not trigger update")
}

func TestFindAsset_Found(t *testing.T) {
	rel := &Release{
		Assets: []Asset{
			{Name: AssetName(), BrowserDownloadURL: "https://example.com/binary"},
		},
	}
	url, err := FindAsset(rel)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/binary", url)
}

func TestFindAsset_NotFound(t *testing.T) {
	rel := &Release{
		Assets: []Asset{
			{Name: "claude-postman-windows-386", BrowserDownloadURL: "https://example.com/win"},
		},
	}
	_, err := FindAsset(rel)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no asset found")
}

func TestAssetName(t *testing.T) {
	name := AssetName()
	assert.Contains(t, name, "claude-postman-")
	// Should contain OS and arch
	assert.True(t, strings.Count(name, "-") >= 2)
}

func TestNormalizeVersion(t *testing.T) {
	assert.Equal(t, "0.1.0", NormalizeVersion("v0.1.0"))
	assert.Equal(t, "0.1.0", NormalizeVersion("0.1.0"))
}
