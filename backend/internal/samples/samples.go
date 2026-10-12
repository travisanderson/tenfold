// Package samples reads the bundled demo logs and their answer keys from a
// folder on disk. Each sample is a pair: <name>.log and <name>.answers.json.
package samples

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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

// Catalog serves the samples in one folder.
type Catalog struct {
	dir string
}

// NewCatalog returns a catalog for the folder. A missing folder is an empty
// catalog, not an error: the API still runs without demo data.
func NewCatalog(dir string) Catalog { return Catalog{dir: dir} }

// validName matches the contract's pattern for sample names. It also keeps
// a name from reaching outside the folder.
var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// copy written for the samples we ship; anything else gets a plain default.
var known = map[string][2]string{
	"attack-day": {"Northwind Labs, attack day", "One day of proxy traffic with four planted attacks and three benign decoys."},
	"clean-day":  {"Northwind Labs, clean day", "One day of normal traffic and three benign decoys. No attacks."},
	"large-100k": {"Northwind Labs, 100,000 lines", "A larger company on the same day, with the same attacks and decoys."},
}

// List returns every sample in the folder, by name.
func (c Catalog) List() ([]Sample, error) {
	paths, err := filepath.Glob(filepath.Join(c.dir, "*.log"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var all []Sample
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".log")
		if s, err := c.Get(name); err == nil {
			all = append(all, s)
		}
	}
	return all, nil
}

// Get describes one sample.
func (c Catalog) Get(name string) (Sample, error) {
	if !validName.MatchString(name) {
		return Sample{}, ErrNotFound
	}
	info, err := os.Stat(c.logPath(name))
	if err != nil {
		return Sample{}, ErrNotFound
	}
	s := Sample{Name: name, Title: name, Description: "A demo log file.", SizeBytes: info.Size()}
	if copy, ok := known[name]; ok {
		s.Title, s.Description = copy[0], copy[1]
	}
	if key, err := c.AnswerKey(name); err == nil {
		s.HasAnswerKey = true
		s.LineCount = key.LineCount
	}
	return s, nil
}

// Open streams a sample's log. The caller closes it.
func (c Catalog) Open(name string) (io.ReadCloser, error) {
	if !validName.MatchString(name) {
		return nil, ErrNotFound
	}
	f, err := os.Open(c.logPath(name))
	if err != nil {
		return nil, ErrNotFound
	}
	return f, nil
}

// AnswerKey reads a sample's answer key.
func (c Catalog) AnswerKey(name string) (AnswerKey, error) {
	if !validName.MatchString(name) {
		return AnswerKey{}, ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(c.dir, name+".answers.json"))
	if err != nil {
		return AnswerKey{}, ErrNotFound
	}
	var key AnswerKey
	if err := json.Unmarshal(data, &key); err != nil {
		return AnswerKey{}, err
	}
	return key, nil
}

func (c Catalog) logPath(name string) string {
	return filepath.Join(c.dir, name+".log")
}
