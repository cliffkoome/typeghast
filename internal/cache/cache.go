// Package cache keeps downloaded data under the user's cache directory so the
// game starts fast and still works offline.
package cache

import (
	"os"
	"path/filepath"
	"time"
)

// Get returns the cached copy of name if it is younger than maxAge. Otherwise
// it calls fetch and stores the result. If fetch fails, a stale copy is
// better than nothing and is returned instead.
func Get(name string, maxAge time.Duration, fetch func() ([]byte, error)) ([]byte, error) {
	path := ""
	if dir, err := os.UserCacheDir(); err == nil {
		path = filepath.Join(dir, "typeghast", name)
	}
	var stale []byte
	if path != "" {
		if info, err := os.Stat(path); err == nil {
			if data, err := os.ReadFile(path); err == nil {
				if time.Since(info.ModTime()) < maxAge {
					return data, nil
				}
				stale = data
			}
		}
	}

	data, err := fetch()
	if err != nil {
		if stale != nil {
			return stale, nil
		}
		return nil, err
	}
	if path != "" && os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		os.WriteFile(path, data, 0o644)
	}
	return data, nil
}
