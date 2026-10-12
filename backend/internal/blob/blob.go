// Package blob stores uploaded log files. The browser sends a file straight
// to storage through a signed URL; the API only signs, reads and deletes.
package blob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotFound means no file is stored under the key, usually because the
// browser never finished its PUT.
var ErrNotFound = errors.New("file not found in storage")

// Store is what the rest of the server needs from file storage.
type Store interface {
	// SignedUploadURL returns a URL the browser can PUT one file to.
	SignedUploadURL(ctx context.Context, key string) (uploadURL string, expiresAt time.Time, err error)
	// Open streams a stored file. The caller closes it.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes a file. Deleting a missing file is not an error.
	Delete(ctx context.Context, key string) error
}

// signedURLLifetime is fixed by Supabase: signed upload URLs last two hours.
const signedURLLifetime = 2 * time.Hour

// Supabase stores files in one private Supabase Storage bucket.
type Supabase struct {
	baseURL string // https://<ref>.supabase.co/storage/v1
	// uploadURL is the same address as the browser reaches it. The two
	// differ only when the server and the browser are on different
	// networks, as in Docker.
	uploadURL string
	key       string // secret key; never leaves the server
	bucket    string
	client    *http.Client
}

// NewSupabase returns a store for one bucket of a Supabase project.
func NewSupabase(projectURL, serviceKey, bucket string) *Supabase {
	base := strings.TrimRight(projectURL, "/") + "/storage/v1"
	return &Supabase{
		baseURL:   base,
		uploadURL: base,
		key:       serviceKey,
		bucket:    bucket,
		// No overall timeout: Open streams files of up to 50 MB.
		client: &http.Client{},
	}
}

// EnsureBucket creates the private bucket if it does not exist yet.
func (s *Supabase) EnsureBucket(ctx context.Context, maxFileBytes int64) error {
	resp, err := s.do(ctx, http.MethodGet, "/bucket/"+url.PathEscape(s.bucket), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	resp, err = s.do(ctx, http.MethodPost, "/bucket", map[string]any{
		"id":              s.bucket,
		"name":            s.bucket,
		"public":          false,
		"file_size_limit": maxFileBytes,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return statusError("create bucket", resp)
	}
	return nil
}

// ReachedFromBrowserAt sets the project URL the browser uses, for when it
// is not the one the server uses. Signed upload URLs are built on it,
// because the browser is who sends the file.
func (s *Supabase) ReachedFromBrowserAt(projectURL string) *Supabase {
	s.uploadURL = strings.TrimRight(projectURL, "/") + "/storage/v1"
	return s
}

func (s *Supabase) SignedUploadURL(ctx context.Context, key string) (string, time.Time, error) {
	resp, err := s.do(ctx, http.MethodPost, "/object/upload/sign/"+s.objectPath(key), map[string]any{})
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, statusError("sign upload URL", resp)
	}
	var body struct {
		URL string `json:"url"` // a path relative to /storage/v1, with ?token=
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.URL == "" {
		return "", time.Time{}, fmt.Errorf("sign upload URL: unexpected response")
	}
	return s.uploadURL + body.URL, time.Now().Add(signedURLLifetime).UTC(), nil
}

func (s *Supabase) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := s.do(ctx, http.MethodGet, "/object/"+s.objectPath(key), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}
	defer resp.Body.Close()
	// Supabase answers 400 with a not-found body for a missing object.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
		return nil, ErrNotFound
	}
	return nil, statusError("read file", resp)
}

func (s *Supabase) Delete(ctx context.Context, key string) error {
	resp, err := s.do(ctx, http.MethodDelete, "/object/"+url.PathEscape(s.bucket),
		map[string]any{"prefixes": []string{key}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError("delete file", resp)
	}
	return nil
}

// List returns the size of every file at the top level of the bucket, by
// name. The demo samples are read this way.
func (s *Supabase) List(ctx context.Context) (map[string]int64, error) {
	resp, err := s.do(ctx, http.MethodPost, "/object/list/"+url.PathEscape(s.bucket), map[string]any{
		"prefix": "", "limit": 1000, "offset": 0,
		"sortBy": map[string]string{"column": "name", "order": "asc"},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("list files", resp)
	}
	var objects []struct {
		Name     string `json:"name"`
		Metadata *struct {
			Size int64 `json:"size"`
		} `json:"metadata"` // null for a folder
	}
	if err := json.NewDecoder(resp.Body).Decode(&objects); err != nil {
		return nil, fmt.Errorf("storage: list files: %w", err)
	}
	sizes := map[string]int64{}
	for _, o := range objects {
		if o.Metadata != nil {
			sizes[o.Name] = o.Metadata.Size
		}
	}
	return sizes, nil
}

// Put uploads a file from the server, replacing any file already there.
// Only tooling uses it: analysts' files go through signed URLs instead.
func (s *Supabase) Put(ctx context.Context, key, contentType string, content io.Reader, size int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/object/"+s.objectPath(key), content)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("apikey", s.key)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError("upload file", resp)
	}
	return nil
}

func (s *Supabase) objectPath(key string) string {
	return url.PathEscape(s.bucket) + "/" + key
}

func (s *Supabase) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("apikey", s.key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	return resp, nil
}

// statusError reports a failed storage call without echoing more than a
// short piece of the response.
func statusError(action string, resp *http.Response) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	return fmt.Errorf("storage: %s: status %d: %s", action, resp.StatusCode, strings.TrimSpace(string(snippet)))
}
