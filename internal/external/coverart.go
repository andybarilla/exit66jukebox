package external

import (
	"context"
	"io"
	"net/http"
)

const coverArtBaseURL = "https://coverartarchive.org"

// CAA fetches front-cover images from the Cover Art Archive.
type CAA struct {
	c       *Client
	baseURL string // overridable in tests
}

// NewCoverArt wraps a rate-limited client.
func NewCoverArt(c *Client) *CAA {
	return &CAA{c: c, baseURL: coverArtBaseURL}
}

// FetchFrontCover downloads the front-cover image for a release. ok is false
// (with no error) when the release has no cover (404); the underlying client
// follows the CAA redirect to the actual image host.
func (a *CAA) FetchFrontCover(ctx context.Context, releaseMBID string) (data []byte, contentType string, ok bool, err error) {
	u := a.baseURL + "/release/" + releaseMBID + "/front"
	resp, err := a.c.do(ctx, u)
	if err != nil {
		return nil, "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", false, nil
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, "", false, err
	}
	contentType = resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, true, nil
}
