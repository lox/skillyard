package state

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	ConfigDir  string
	DataDir    string
	CacheDir   string
	SourcesDir string
	ConfigPath string
	LockPath   string
}

func DefaultPaths() (Paths, error) {
	configDir, err := userConfigDir()
	if err != nil {
		return Paths{}, err
	}
	dataDir, err := userDataDir()
	if err != nil {
		return Paths{}, err
	}
	cacheDir, err := userCacheDir()
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		ConfigDir:  configDir,
		DataDir:    dataDir,
		CacheDir:   cacheDir,
		SourcesDir: filepath.Join(dataDir, "sources"),
		ConfigPath: filepath.Join(configDir, "config.hcl"),
		LockPath:   filepath.Join(configDir, "skillyard.lock.json"),
	}, nil
}

func userConfigDir() (string, error) {
	if override := os.Getenv("SKILLYARD_CONFIG_DIR"); override != "" {
		return filepath.Clean(override), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, "skillyard"), nil
}

func userDataDir() (string, error) {
	if override := os.Getenv("SKILLYARD_DATA_DIR"); override != "" {
		return filepath.Clean(override), nil
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "skillyard"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "skillyard"), nil
}

func userCacheDir() (string, error) {
	if override := os.Getenv("SKILLYARD_CACHE_DIR"); override != "" {
		return filepath.Clean(override), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(base, "skillyard"), nil
}

func (p Paths) Ensure() error {
	for _, dir := range []string{p.ConfigDir, p.SourcesDir, p.CacheDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}
