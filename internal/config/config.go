package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"gopkg.in/yaml.v3"
)

type ConnectorType string

const (
	ConnectorJellyfin ConnectorType = "jellyfin"
	ConnectorEmby     ConnectorType = "emby"
	ConnectorPlex     ConnectorType = "plex"
	ConnectorPlexTV   ConnectorType = "plextv"
)

type ServerConfig struct {
	Alias     string        `yaml:"alias"                json:"alias"`
	Type      ConnectorType `yaml:"type"                 json:"type"`
	URL       string        `yaml:"url"                  json:"url"`
	Username  string        `yaml:"username"             json:"username"`
	APIKey    string        `yaml:"api_key,omitempty"    json:"api_key,omitempty"`
	Password  string        `yaml:"password,omitempty"   json:"password,omitempty"`
	MachineID string        `yaml:"machine_id,omitempty" json:"machine_id,omitempty"` // Plex server machine identifier (plextv only)
	Enabled   bool          `yaml:"enabled"              json:"enabled"`
}

// DownloadConfig controls both parallel download and streaming read-ahead.
// ChunkSizeMB is the fundamental unit: one chunk = one HTTP Range request = one in-memory window.
// ParallelChunks controls both download concurrency and the number of prefetch windows per file.
type DownloadConfig struct {
	ParallelChunks int `yaml:"parallel_chunks"` // parallel Range requests; also = read-ahead window count
	ChunkSizeMB    int `yaml:"chunk_size_mb"`   // size of each chunk/window in MB
	BufferSizeKB   int `yaml:"buffer_size_kb"`  // internal I/O read buffer
	MaxCacheMB     int `yaml:"max_cache_mb"`    // total in-memory window cache limit
	CacheTTLMin    int `yaml:"cache_ttl_min"`   // eviction TTL for cached windows

	// Deprecated: ignored, kept for config file backward-compatibility.
	ReadAheadWindowMB int `yaml:"read_ahead_window_mb,omitempty"`
	ReadAheadWindows  int `yaml:"read_ahead_windows,omitempty"`
}

type CacheConfig struct {
	TTLItemsSec    int `yaml:"ttl_items_sec"`
	TTLMetaSec     int `yaml:"ttl_metadata_sec"`
	TTLArtworkSec  int `yaml:"ttl_artwork_sec"`
}

type MountConfig struct {
	DriveLetter     string `yaml:"drive_letter"`      // Windows
	MountPoint      string `yaml:"mount_point"`       // Linux
	Options         string `yaml:"options,omitempty"` // Linux FUSE options (e.g. "ro,allow_other")
	VirtualSidecars bool   `yaml:"virtual_sidecars"`  // Generate .nfo and artwork sidecars
}

type Config struct {
	Mount    MountConfig    `yaml:"mount"`
	Servers  []ServerConfig `yaml:"servers"`
	Download DownloadConfig `yaml:"download"`
	Cache    CacheConfig    `yaml:"cache"`
}

func DefaultConfig() *Config {
	return &Config{
		Mount: MountConfig{
			DriveLetter: "Z",
			MountPoint:  "/mnt/jellyfin-remote",
			Options:     "ro,allow_other",
		},
		Download: DownloadConfig{
			ParallelChunks: 2,
			ChunkSizeMB:    16,
			BufferSizeKB:   256,
			MaxCacheMB:     512,
			CacheTTLMin:    30,
		},
		Cache: CacheConfig{
			TTLItemsSec:   604800,  // 7 gün (varsayılan: 604800s)
			TTLMetaSec:    2592000, // 30 gün
			TTLArtworkSec: 2592000, // 30 gün
		},
	}
}

func ConfigDir() string {
	if d := getEnvAny("MEDIAFS_CONFIG_DIR", "CONFIG_DIR"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "MediaFS")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "mediafs")
}

func ConfigPath() string {
	if p := getEnvAny("MEDIAFS_CONFIG_PATH", "CONFIG_PATH"); p != "" {
		return p
	}
	return filepath.Join(ConfigDir(), "config.yaml")
}

func Load() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	cfg := DefaultConfig()
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := getEnvAny("MEDIAFS_MOUNT_POINT", "MOUNT_POINT"); v != "" {
		cfg.Mount.MountPoint = v
	}
	if v := getEnvAny("MEDIAFS_MOUNT_OPTIONS", "MOUNT_OPTIONS"); v != "" {
		cfg.Mount.Options = v
	}
	if v := getEnvAny("MEDIAFS_DRIVE_LETTER", "DRIVE_LETTER"); v != "" {
		cfg.Mount.DriveLetter = v
	}
	if v := getEnvAny("MEDIAFS_VIRTUAL_SIDECARS", "VIRTUAL_SIDECARS"); v != "" {
		cfg.Mount.VirtualSidecars = strings.EqualFold(v, "true") || v == "1"
	}

	jellyURL := getEnvAny("JELLYFIN_URL", "MEDIAFS_JELLYFIN_URL")
	if jellyURL != "" {
		username := getEnvAny("JELLYFIN_USERNAME", "MEDIAFS_JELLYFIN_USERNAME")
		password := getEnvAny("JELLYFIN_PASSWORD", "MEDIAFS_JELLYFIN_PASSWORD")
		apiKey := getEnvAny("JELLYFIN_API_KEY", "MEDIAFS_JELLYFIN_API_KEY")
		alias := getEnvAny("JELLYFIN_ALIAS", "MEDIAFS_JELLYFIN_ALIAS")
		if alias == "" {
			alias = "Jellyfin"
		}

		found := false
		for i := range cfg.Servers {
			if cfg.Servers[i].Type == ConnectorJellyfin && (cfg.Servers[i].URL == jellyURL || cfg.Servers[i].Alias == alias) {
				cfg.Servers[i].URL = jellyURL
				if username != "" {
					cfg.Servers[i].Username = username
				}
				if password != "" {
					cfg.Servers[i].Password = password
				}
				if apiKey != "" {
					cfg.Servers[i].APIKey = apiKey
				}
				cfg.Servers[i].Enabled = true
				found = true
				break
			}
		}
		if !found {
			cfg.Servers = append(cfg.Servers, ServerConfig{
				Alias:    alias,
				Type:     ConnectorJellyfin,
				URL:      jellyURL,
				Username: username,
				Password: password,
				APIKey:   apiKey,
				Enabled:  true,
			})
		}
	}

	if v := getEnvAny("MEDIAFS_PARALLEL_CHUNKS", "PARALLEL_CHUNKS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Download.ParallelChunks = n
		}
	}
	if v := getEnvAny("MEDIAFS_CHUNK_SIZE_MB", "CHUNK_SIZE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Download.ChunkSizeMB = n
		}
	}
	if v := getEnvAny("MEDIAFS_BUFFER_SIZE_KB", "BUFFER_SIZE_KB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Download.BufferSizeKB = n
		}
	}
	if v := getEnvAny("MEDIAFS_MAX_CACHE_MB", "MAX_CACHE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Download.MaxCacheMB = n
		}
	}
	if v := getEnvAny("MEDIAFS_CACHE_TTL_MIN", "CACHE_TTL_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Download.CacheTTLMin = n
		}
	}
	if v := getEnvAny("TTL_ITEMS_SEC", "CACHE_TTL_ITEMS_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Cache.TTLItemsSec = n
		}
	}
	if v := getEnvAny("TTL_META_SEC", "CACHE_TTL_META_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Cache.TTLMetaSec = n
		}
	}
}

func getEnvAny(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func (c *Config) Save() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), data, 0600)
}

// ServerKey returns the display name used as the VFS root folder name.
// Format: "username@alias"
func (s ServerConfig) ServerKey() string {
	if s.Username != "" {
		return s.Username + "@" + s.Alias
	}
	return s.Alias
}
