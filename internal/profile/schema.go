package profile

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schemas/*.json
var pinned embed.FS

// Schemas validates config against JSON Schemas. Pinned copies ship with the
// bundled profiles; other schemas are fetched once into Cache.
type Schemas struct {
	Cache    string // Fetching is disabled when empty.
	Fetch    func(url string) ([]byte, error)
	compiled map[string]*jsonschema.Schema
}

// NewSchemas caches fetched schemas below the user cache directory.
func NewSchemas() *Schemas {
	s := &Schemas{Fetch: fetch}
	if dir, err := os.UserCacheDir(); err == nil {
		s.Cache = filepath.Join(dir, "henia", "schemas")
	}
	return s
}

func (s *Schemas) Validate(url string, value any) error {
	schema, err := s.schema(url)
	if err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return schema.Validate(instance)
}

func (s *Schemas) schema(url string) (*jsonschema.Schema, error) {
	if schema, ok := s.compiled[url]; ok {
		return schema, nil
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(loader{s})
	schema, err := compiler.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("schema %s unavailable: %w", url, err)
	}
	if s.compiled == nil {
		s.compiled = map[string]*jsonschema.Schema{}
	}
	s.compiled[url] = schema
	return schema, nil
}

type loader struct{ s *Schemas }

func (l loader) Load(url string) (any, error) {
	data, err := l.s.load(url)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(data))
}

func (s *Schemas) load(url string) ([]byte, error) {
	name := cacheName(url)
	if data, err := pinned.ReadFile("schemas/" + name); err == nil {
		return data, nil
	}
	if s.Cache == "" || s.Fetch == nil {
		return nil, fmt.Errorf("not pinned and fetching is disabled")
	}
	path := filepath.Join(s.Cache, name)
	if data, err := os.ReadFile(path); err == nil {
		return data, nil
	}
	data, err := s.Fetch(url)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.Cache, 0o755); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
	return data, nil
}

func cacheName(url string) string {
	name := url[strings.Index(url, "://")+1:]
	name = strings.Trim(strings.ReplaceAll(name, "/", "_"), "_")
	name = strings.TrimSuffix(name, ".json")
	sum := sha256.Sum256([]byte(url))
	return name + "-" + hex.EncodeToString(sum[:4]) + ".json"
}

func fetch(url string) ([]byte, error) {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return nil, fmt.Errorf("unsupported schema location")
	}
	client := http.Client{Timeout: 15 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch: %s", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 16<<20))
}
