package config

import (
	"strconv"
	"strings"
	"testing"
)

func TestEnterpriseSurfaceDefaultsOffAndValidatesSwitch(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil || cfg.EnterpriseEnabled {
		t.Fatal("enterprise enabled by default", err)
	}
	for _, raw := range []string{"true", "false"} {
		cfg, err := load(envFunc(map[string]string{keyEnterpriseEnabled: raw}))
		if err != nil || cfg.EnterpriseEnabled != (raw == "true") {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"", "secret-value", " true "} {
		if _, err := load(envFunc(map[string]string{keyEnterpriseEnabled: raw})); err == nil || !strings.Contains(err.Error(), keyEnterpriseEnabled) || strings.Contains(err.Error(), "secret-value") {
			t.Fatal("unsafe config error", err)
		}
	}
}

func TestEnterpriseDeviceLimitIsServerOwnedAndBounded(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil || cfg.EnterpriseDeviceLimit != 3 {
		t.Fatal("default device limit", err)
	}
	for _, count := range []int64{1, 2, 3, 4, 5} {
		cfg, err := load(envFunc(map[string]string{keyEnterpriseDeviceLimit: strconv.FormatInt(count, 10)}))
		if err != nil || cfg.EnterpriseDeviceLimit != count {
			t.Fatal("valid device limit", count, err)
		}
	}
	for _, raw := range []string{"", "0", "6", "-1", "3.0", " 3 ", "private-invalid-value"} {
		if _, err := load(envFunc(map[string]string{keyEnterpriseDeviceLimit: raw})); err == nil || !strings.Contains(err.Error(), keyEnterpriseDeviceLimit) || strings.Contains(err.Error(), "private-invalid-value") {
			t.Fatal("invalid device limit error was missing or disclosed input", err)
		}
	}
}

// EMP-METER-003 (EDR-185): the reading validation is on unless the operator
// turns it off; the switch is a strict boolean and never echoes its input.
func TestEnterpriseMeterPlausibilityDefaultsOnAndValidatesSwitch(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil || !cfg.EnterpriseMeterPlausibility {
		t.Fatal("meter plausibility off by default", err)
	}
	for _, raw := range []string{"true", "false"} {
		cfg, err := load(envFunc(map[string]string{keyEnterpriseMeterPlausibility: raw}))
		if err != nil || cfg.EnterpriseMeterPlausibility != (raw == "true") {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"", "off", " true ", "private-invalid-value"} {
		if _, err := load(envFunc(map[string]string{keyEnterpriseMeterPlausibility: raw})); err == nil || !strings.Contains(err.Error(), keyEnterpriseMeterPlausibility) || strings.Contains(err.Error(), "private-invalid-value") {
			t.Fatal("invalid meter plausibility switch accepted or disclosed", err)
		}
	}
}
