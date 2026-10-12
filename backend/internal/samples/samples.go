// Package samples serves the bundled demo logs and their answer keys. Each
// sample is a pair of files: <name>.log and <name>.answers.json.
//
// In production the files live in a private storage bucket, which keeps
// about 90 MB of generated logs out of git and out of the server image.
// Tests read the same files from a folder on disk.
package samples

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound means there is no sample with that name.
var ErrNotFound = errors.New("sample not found")

// Sample describes one demo log.
type Sample struct {
	Name         string
	Title        string
	Description  string
	LineCount    int
	SizeBytes    int64
	HasAnswerKey bool
}

// AnswerKey lists what the generator planted in a sample.
type AnswerKey struct {
	Sample         string     `json:"sample"`
	LineCount      int        `json:"lineCount"`
	MalformedLines []int      `json:"malformedLines"`
	Scenarios      []Scenario `json:"scenarios"`
}

// Scenario is one planted attack or benign decoy.
type Scenario struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"` // "malicious" or "benign"
	Type      string   `json:"type"`
	Mitre     []string `json:"mitre"`
	Users     []string `json:"users"`
	Hosts     []string `json:"hosts"`
	Domains   []string `json:"domains"`
	TimeRange struct {
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
	} `json:"timeRange"`
	LineNos []int `json:"lineNos"`
}

// Malicious reports whether the scenario is a planted attack.
func (s Scenario) Malicious() bool { return s.Label == "malicious" }

// Catalog is where the server finds samples.
type Catalog interface {
	// List returns every sample, by name.
	List(ctx context.Context) ([]Sample, error)
	// Get describes one sample, or returns ErrNotFound.
	Get(ctx context.Context, name string) (Sample, error)
	// Open streams a sample's log. The caller closes it.
	Open(ctx context.Context, name string) (io.ReadCloser, error)
	// AnswerKey reads a sample's answer key, or returns ErrNotFound.
	AnswerKey(ctx context.Context, name string) (AnswerKey, error)
}

// Files is the storage a catalog reads from. A Supabase bucket and a
// folder on disk both fit.
type Files interface {
	// List returns the size of every file, by file name.
	List(ctx context.Context) (map[string]int64, error)
	// Open streams one file. A missing file is an error.
	Open(ctx context.Context, name string) (io.ReadCloser, error)
}

const (
	logSuffix = ".log"
	keySuffix = ".answers.json"

	// listTTL is how long a listing is trusted. The files change only
	// when someone uploads a new set, so a short cache saves a storage
	// call on nearly every request.
	listTTL = 5 * time.Minute
)

// validName matches the contract's pattern for sample names. It also keeps
// a name from reaching outside the folder or bucket.
var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// copy written for the samples we ship; anything else gets a plain default.
var known = map[string][2]string{
	"attack-day": {"Northwind Labs, attack day", "One day of proxy traffic with four planted attacks and three benign decoys."},
	"clean-day":  {"Northwind Labs, clean day", "One day of normal traffic and three benign decoys. No attacks."},
	"large-100k": {"Northwind Labs, 100,000 lines", "A larger company on the same day, with the same attacks and decoys."},
}

// catalog serves samples from any Files, remembering what it has read.
type catalog struct {
	files Files

	mu       sync.Mutex
	listed   []Sample
	listedAt time.Time
	keys     map[string]AnswerKey
}

// NewCatalog returns a catalog over some file storage.
func NewCatalog(files Files) Catalog {
	return &catalog{files: files, keys: map[string]AnswerKey{}}
}

func (c *catalog) List(ctx context.Context) ([]Sample, error) {
	c.mu.Lock()
	if c.listed != nil && time.Since(c.listedAt) < listTTL {
		defer c.mu.Unlock()
		return c.listed, nil
	}
	c.mu.Unlock()

	sizes, err := c.files.List(ctx)
	if err != nil {
		return nil, err
	}
	all := []Sample{}
	for file, size := range sizes {
		name, isLog := strings.CutSuffix(file, logSuffix)
		if !isLog || !validName.MatchString(name) {
			continue
		}
		s := Sample{Name: name, Title: name, Description: "A demo log file.", SizeBytes: size}
		if text, ok := known[name]; ok {
			s.Title, s.Description = text[0], text[1]
		}
		if _, hasKey := sizes[name+keySuffix]; hasKey {
			// The line count comes from the answer key. If the key cannot
			// be read, the sample is still usable; it just has no scorecard.
			if key, err := c.AnswerKey(ctx, name); err == nil {
				s.HasAnswerKey, s.LineCount = true, key.LineCount
			}
		}
		all = append(all, s)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	c.mu.Lock()
	c.listed, c.listedAt = all, time.Now()
	c.mu.Unlock()
	return all, nil
}

func (c *catalog) Get(ctx context.Context, name string) (Sample, error) {
	if !validName.MatchString(name) {
		return Sample{}, ErrNotFound
	}
	all, err := c.List(ctx)
	if err != nil {
		return Sample{}, err
	}
	for _, s := range all {
		if s.Name == name {
			return s, nil
		}
	}
	return Sample{}, ErrNotFound
}

func (c *catalog) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	if _, err := c.Get(ctx, name); err != nil {
		return nil, err
	}
	return c.files.Open(ctx, name+logSuffix)
}

func (c *catalog) AnswerKey(ctx context.Context, name string) (AnswerKey, error) {
	if !validName.MatchString(name) {
		return AnswerKey{}, ErrNotFound
	}
	c.mu.Lock()
	key, cached := c.keys[name]
	c.mu.Unlock()
	if cached {
		return key, nil
	}

	f, err := c.files.Open(ctx, name+keySuffix)
	if err != nil {
		return AnswerKey{}, ErrNotFound
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&key); err != nil {
		return AnswerKey{}, err
	}
	c.mu.Lock()
	c.keys[name] = key
	c.mu.Unlock()
	return key, nil
}

// Dir is a folder of sample files on disk. A missing folder is an empty
// one, so tests skip cleanly when the samples have not been generated.
type Dir string

func (d Dir) List(context.Context) (map[string]int64, error) {
	entries, err := os.ReadDir(string(d))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]int64{}, nil
	}
	if err != nil {
		return nil, err
	}
	sizes := map[string]int64{}
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
			sizes[entry.Name()] = info.Size()
		}
	}
	return sizes, nil
}

func (d Dir) Open(_ context.Context, name string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(string(d), filepath.Base(name)))
}
