package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/theQRL/qrysm/testing/require"
)

func TestGet_ResponseBodyLimits(t *testing.T) {
	const limit int64 = 64
	for _, tc := range []struct {
		name          string
		size          int64
		contentLength int64
		wantRead      int64
		wantErr       bool
	}{
		{name: "empty", size: 0, contentLength: 0, wantRead: 0},
		{name: "below limit", size: limit - 1, contentLength: limit - 1, wantRead: limit - 1},
		{name: "at limit", size: limit, contentLength: limit, wantRead: limit},
		{name: "at limit without content length", size: limit, contentLength: -1, wantRead: limit},
		{name: "oversized content length", size: limit + 1, contentLength: limit + 1, wantErr: true},
		{name: "oversized chunked body", size: 2 * limit, contentLength: -1, wantRead: limit + 1, wantErr: true},
		{name: "underreported content length", size: 2 * limit, contentLength: 1, wantRead: limit + 1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedResponseBody{Reader: io.LimitReader(zeroResponseBody{}, tc.size)}
			c := clientWithResponse(t, &http.Response{
				StatusCode: http.StatusOK, ContentLength: tc.contentLength, Body: body,
			}, WithMaxBodySize(limit))
			got, err := c.Get(context.Background(), "/state")
			if tc.wantErr {
				require.ErrorIs(t, err, ErrResponseTooLarge)
				require.ErrorContains(t, "limit is 64 bytes", err)
				require.Equal(t, true, got == nil)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.size, int64(len(got)))
			}
			require.Equal(t, tc.wantRead, body.read)
			require.Equal(t, true, body.closed)
		})
	}
}

func TestGet_DefaultResponseBodyLimit(t *testing.T) {
	body := &trackedResponseBody{Reader: zeroResponseBody{}}
	c := clientWithResponse(t, &http.Response{
		StatusCode: http.StatusOK, ContentLength: MaxBodySize + 1, Body: body,
	})
	require.Equal(t, int64(8<<20), c.maxBodySize)
	got, err := c.Get(context.Background(), "/state")
	require.ErrorIs(t, err, ErrResponseTooLarge)
	require.Equal(t, true, got == nil)
	require.Equal(t, int64(0), body.read)
	require.Equal(t, true, body.closed)
}

func TestGet_ErrorResponseBodyLimit(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := &trackedResponseBody{Reader: zeroResponseBody{}}
			c := clientWithResponse(t, &http.Response{
				StatusCode: status, ContentLength: 1 << 30, Body: body,
			}, WithMaxBodySize(1<<29))
			got, err := c.Get(context.Background(), "/state")
			require.ErrorIs(t, err, ErrNotOK)
			if status == http.StatusNotFound {
				require.ErrorIs(t, err, ErrNotFound)
			}
			require.Equal(t, true, got == nil)
			require.Equal(t, MaxErrBodySize, body.read)
			require.Equal(t, true, body.closed)
		})
	}
}

func TestGet_ResponseReadError(t *testing.T) {
	for _, prefix := range []string{"", strings.Repeat("x", 64)} {
		t.Run(prefix, func(t *testing.T) {
			body := &trackedResponseBody{Reader: io.MultiReader(strings.NewReader(prefix), failedResponseBody{})}
			c := clientWithResponse(t, &http.Response{
				StatusCode: http.StatusOK, ContentLength: -1, Body: body,
			}, WithMaxBodySize(64))
			got, err := c.Get(context.Background(), "/state")
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			require.Equal(t, true, got == nil)
			require.Equal(t, true, body.closed)
		})
	}
}

func clientWithResponse(t *testing.T, response *http.Response, opts ...ClientOpt) *Client {
	t.Helper()
	opts = append(opts, WithRoundTripper(responseRoundTripper(func(req *http.Request) (*http.Response, error) {
		response.Request = req
		return response, nil
	})))
	c, err := NewClient("http://beacon.invalid", opts...)
	require.NoError(t, err)
	return c
}

type responseRoundTripper func(*http.Request) (*http.Response, error)

func (rt responseRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

type trackedResponseBody struct {
	io.Reader
	read   int64
	closed bool
}

func (b *trackedResponseBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += int64(n)
	return n, err
}

func (b *trackedResponseBody) Close() error {
	b.closed = true
	return nil
}

type zeroResponseBody struct{}

func (zeroResponseBody) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type failedResponseBody struct{}

func (failedResponseBody) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
