package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Mount.MountPoint != "/mnt/jellyfin-remote" {
		t.Errorf("expected default MountPoint /mnt/jellyfin-remote, got %s", cfg.Mount.MountPoint)
	}
	if cfg.Mount.Options != "ro,allow_other" {
		t.Errorf("expected default Options ro,allow_other, got %s", cfg.Mount.Options)
	}
}

func TestEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("MEDIAFS_CONFIG_PATH", filepath.Join(tmpDir, "nonexistent.yaml"))
	os.Setenv("JELLYFIN_URL", "http://jellyfin.local:8096")
	os.Setenv("JELLYFIN_USERNAME", "testuser")
	os.Setenv("JELLYFIN_PASSWORD", "testpass")
	os.Setenv("JELLYFIN_ALIAS", "HomeJelly")
	os.Setenv("MOUNT_POINT", "/mnt/custom-jellyfin")
	os.Setenv("CHUNK_SIZE_MB", "32")
	os.Setenv("MAX_CACHE_MB", "1024")
	defer func() {
		os.Unsetenv("MEDIAFS_CONFIG_PATH")
		os.Unsetenv("JELLYFIN_URL")
		os.Unsetenv("JELLYFIN_USERNAME")
		os.Unsetenv("JELLYFIN_PASSWORD")
		os.Unsetenv("JELLYFIN_ALIAS")
		os.Unsetenv("MOUNT_POINT")
		os.Unsetenv("CHUNK_SIZE_MB")
		os.Unsetenv("MAX_CACHE_MB")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Mount.MountPoint != "/mnt/custom-jellyfin" {
		t.Errorf("expected /mnt/custom-jellyfin, got %s", cfg.Mount.MountPoint)
	}
	if cfg.Download.ChunkSizeMB != 32 {
		t.Errorf("expected ChunkSizeMB 32, got %d", cfg.Download.ChunkSizeMB)
	}
	if cfg.Download.MaxCacheMB != 1024 {
		t.Errorf("expected MaxCacheMB 1024, got %d", cfg.Download.MaxCacheMB)
	}

	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	srv := cfg.Servers[0]
	if srv.Type != ConnectorJellyfin {
		t.Errorf("expected type jellyfin, got %s", srv.Type)
	}
	if srv.URL != "http://jellyfin.local:8096" {
		t.Errorf("expected URL http://jellyfin.local:8096, got %s", srv.URL)
	}
	if srv.Username != "testuser" || srv.Password != "testpass" {
		t.Errorf("expected username testuser and password testpass, got %s/%s", srv.Username, srv.Password)
	}
	if srv.Alias != "HomeJelly" {
		t.Errorf("expected alias HomeJelly, got %s", srv.Alias)
	}
}
