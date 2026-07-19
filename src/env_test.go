package main

import (
	"testing"

	"github.com/caarlos0/env/v11"
)

func parseDevicePropsConfig(t *testing.T) DevicePropsConfig {
	t.Helper()
	cfg := DevicePropsConfig{}
	if err := env.Parse(&cfg); err != nil {
		t.Fatalf("env.Parse: %v", err)
	}
	return cfg
}

func TestRequireFullSync_NotSet(t *testing.T) {
	cfg := parseDevicePropsConfig(t)
	if cfg.RequireFullSync.Set {
		t.Fatal("expected Set=false when env is absent")
	}
}

func TestRequireFullSync_EmptyString(t *testing.T) {
	// caarlos0/env skips UnmarshalText for empty strings — treated as noop
	t.Setenv("WAHA_GOWS_DEVICE_REQUIRE_FULL_SYNC", "")
	cfg := parseDevicePropsConfig(t)
	if cfg.RequireFullSync.Set {
		t.Fatal("expected Set=false for empty string (noop)")
	}
}

func TestRequireFullSync_Null(t *testing.T) {
	t.Setenv("WAHA_GOWS_DEVICE_REQUIRE_FULL_SYNC", "null")
	cfg := parseDevicePropsConfig(t)
	if !cfg.RequireFullSync.Set {
		t.Fatal("expected Set=true")
	}
	if cfg.RequireFullSync.Value != nil {
		t.Fatalf("expected Value=nil, got %v", cfg.RequireFullSync.Value)
	}
}

func TestRequireFullSync_True(t *testing.T) {
	t.Setenv("WAHA_GOWS_DEVICE_REQUIRE_FULL_SYNC", "true")
	cfg := parseDevicePropsConfig(t)
	if !cfg.RequireFullSync.Set {
		t.Fatal("expected Set=true")
	}
	if cfg.RequireFullSync.Value == nil || *cfg.RequireFullSync.Value != true {
		t.Fatalf("expected Value=true, got %v", cfg.RequireFullSync.Value)
	}
}

func TestRequireFullSync_False(t *testing.T) {
	t.Setenv("WAHA_GOWS_DEVICE_REQUIRE_FULL_SYNC", "false")
	cfg := parseDevicePropsConfig(t)
	if !cfg.RequireFullSync.Set {
		t.Fatal("expected Set=true")
	}
	if cfg.RequireFullSync.Value == nil || *cfg.RequireFullSync.Value != false {
		t.Fatalf("expected Value=false, got %v", cfg.RequireFullSync.Value)
	}
}

func TestRequireFullSync_One(t *testing.T) {
	t.Setenv("WAHA_GOWS_DEVICE_REQUIRE_FULL_SYNC", "1")
	cfg := parseDevicePropsConfig(t)
	if cfg.RequireFullSync.Value == nil || *cfg.RequireFullSync.Value != true {
		t.Fatalf("expected Value=true, got %v", cfg.RequireFullSync.Value)
	}
}

func TestRequireFullSync_Zero(t *testing.T) {
	t.Setenv("WAHA_GOWS_DEVICE_REQUIRE_FULL_SYNC", "0")
	cfg := parseDevicePropsConfig(t)
	if cfg.RequireFullSync.Value == nil || *cfg.RequireFullSync.Value != false {
		t.Fatalf("expected Value=false, got %v", cfg.RequireFullSync.Value)
	}
}

func TestFullSyncDaysLimit_NotSet(t *testing.T) {
	cfg := parseDevicePropsConfig(t)
	if cfg.FullSyncDaysLimit.Set {
		t.Fatal("expected Set=false when env is absent")
	}
}

func TestFullSyncDaysLimit_EmptyString(t *testing.T) {
	// caarlos0/env skips UnmarshalText for empty strings — treated as noop
	t.Setenv("WAHA_GOWS_DEVICE_HISTORY_SYNC_FULL_SYNC_DAYS_LIMIT", "")
	cfg := parseDevicePropsConfig(t)
	if cfg.FullSyncDaysLimit.Set {
		t.Fatal("expected Set=false for empty string (noop)")
	}
}

func TestFullSyncDaysLimit_Null(t *testing.T) {
	t.Setenv("WAHA_GOWS_DEVICE_HISTORY_SYNC_FULL_SYNC_DAYS_LIMIT", "null")
	cfg := parseDevicePropsConfig(t)
	if !cfg.FullSyncDaysLimit.Set {
		t.Fatal("expected Set=true")
	}
	if cfg.FullSyncDaysLimit.Value != nil {
		t.Fatalf("expected Value=nil, got %v", cfg.FullSyncDaysLimit.Value)
	}
}

func TestFullSyncDaysLimit_Value(t *testing.T) {
	t.Setenv("WAHA_GOWS_DEVICE_HISTORY_SYNC_FULL_SYNC_DAYS_LIMIT", "30")
	cfg := parseDevicePropsConfig(t)
	if !cfg.FullSyncDaysLimit.Set {
		t.Fatal("expected Set=true")
	}
	if cfg.FullSyncDaysLimit.Value == nil || *cfg.FullSyncDaysLimit.Value != 30 {
		t.Fatalf("expected Value=30, got %v", cfg.FullSyncDaysLimit.Value)
	}
}

func TestKeepAliveConfig_NotSet(t *testing.T) {
	cfg := getKeepAliveConfig()
	if cfg.IntervalMinSec != 0 || cfg.IntervalMaxSec != 0 {
		t.Fatalf("expected 0/0 when unset, got %d/%d", cfg.IntervalMinSec, cfg.IntervalMaxSec)
	}
}

func TestKeepAliveConfig_Values(t *testing.T) {
	t.Setenv("WAHA_GOWS_KEEPALIVE_INTERVAL_MIN_SEC", "8")
	t.Setenv("WAHA_GOWS_KEEPALIVE_INTERVAL_MAX_SEC", "12")
	cfg := getKeepAliveConfig()
	if cfg.IntervalMinSec != 8 {
		t.Fatalf("expected min=8, got %d", cfg.IntervalMinSec)
	}
	if cfg.IntervalMaxSec != 12 {
		t.Fatalf("expected max=12, got %d", cfg.IntervalMaxSec)
	}
}
