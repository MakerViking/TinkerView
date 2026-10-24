package go2rtc

import (
	"context"
	"net/http"
	"net/url"
)

// PatchStream updates the source of an existing stream in place (PATCH
// /api/streams?name=&src=), keeping the stream object and its consumers. The
// dashboard uses it to apply an edited Frigate-config stream live, without a
// Frigate restart. (PutStream, by contrast, replaces the stream.)
func (c *Client) PatchStream(ctx context.Context, name, src string) error {
	q := url.Values{"name": {name}, "src": {src}}
	return c.do(ctx, http.MethodPatch, q)
}
