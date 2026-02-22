package tui

import (
	"testing"
	"time"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
)

func TestSessionStalenessTier(t *testing.T) {
	m := sessionsModel{
		freshThreshold: 24 * time.Hour,
		staleThreshold: 48 * time.Hour,
	}

	now := time.Now()
	tests := []struct {
		name     string
		activity int64
		want     stalenessTier
	}{
		{"fresh - 5 minutes ago", now.Add(-5 * time.Minute).Unix(), tierFresh},
		{"fresh - 12 hours ago", now.Add(-12 * time.Hour).Unix(), tierFresh},
		{"getting stale - 30 hours ago", now.Add(-30 * time.Hour).Unix(), tierGettingStale},
		{"getting stale - 47 hours ago", now.Add(-47 * time.Hour).Unix(), tierGettingStale},
		{"stale - 49 hours ago", now.Add(-49 * time.Hour).Unix(), tierStale},
		{"stale - 7 days ago", now.Add(-7 * 24 * time.Hour).Unix(), tierStale},
		{"zero timestamp", 0, tierFresh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.sessionStalenessTier(tt.activity)
			if got != tt.want {
				t.Errorf("sessionStalenessTier(%d) = %d, want %d", tt.activity, got, tt.want)
			}
		})
	}
}

func TestSessionStalenessTierDisabled(t *testing.T) {
	m := sessionsModel{
		stalenessDisabled: true,
		freshThreshold:    24 * time.Hour,
		staleThreshold:    48 * time.Hour,
	}

	now := time.Now()
	tests := []struct {
		name     string
		activity int64
	}{
		{"fresh activity", now.Add(-5 * time.Minute).Unix()},
		{"old activity", now.Add(-48 * time.Hour).Unix()},
		{"zero timestamp", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.sessionStalenessTier(tt.activity)
			if got != tierFresh {
				t.Errorf("sessionStalenessTier(%d) with disabled = %d, want tierFresh", tt.activity, got)
			}
		})
	}
}

func TestStaleSessions(t *testing.T) {
	now := time.Now()
	m := sessionsModel{
		freshThreshold: 24 * time.Hour,
		staleThreshold: 48 * time.Hour,
		lines: []tmux.SessionLine{
			{Name: "fresh-session", Activity: now.Add(-10 * time.Minute).Unix()},
			{Name: "medium-session", Activity: now.Add(-30 * time.Hour).Unix()},
			{Name: "stale-session-1", Activity: now.Add(-72 * time.Hour).Unix()},
			{Name: "stale-session-2", Activity: now.Add(-96 * time.Hour).Unix()},
		},
	}

	stale := m.staleSessions()
	if len(stale) != 2 {
		t.Fatalf("staleSessions() returned %d sessions, want 2", len(stale))
	}
	if stale[0] != "stale-session-1" {
		t.Errorf("stale[0] = %q, want %q", stale[0], "stale-session-1")
	}
	if stale[1] != "stale-session-2" {
		t.Errorf("stale[1] = %q, want %q", stale[1], "stale-session-2")
	}

	count := m.staleSessionCount()
	if count != 2 {
		t.Errorf("staleSessionCount() = %d, want 2", count)
	}
}

func TestStalenessConfigDefaults(t *testing.T) {
	// nil config returns defaults
	var c *config.StalenessConfig
	fresh, stale := c.ParsedStalenessThresholds()
	if fresh != 24*time.Hour {
		t.Errorf("default fresh = %v, want 24h", fresh)
	}
	if stale != 48*time.Hour {
		t.Errorf("default stale = %v, want 48h", stale)
	}
	threshold := c.EffectiveSuggestionThreshold()
	if threshold != 7 {
		t.Errorf("default suggestion threshold = %d, want 7", threshold)
	}
}

func TestStalenessConfigCustomValues(t *testing.T) {
	c := &config.StalenessConfig{
		FreshDuration:       "30m",
		StaleDuration:       "2h30m",
		SuggestionThreshold: 5,
	}
	fresh, stale := c.ParsedStalenessThresholds()
	if fresh != 30*time.Minute {
		t.Errorf("custom fresh = %v, want 30m", fresh)
	}
	if stale != 2*time.Hour+30*time.Minute {
		t.Errorf("custom stale = %v, want 2h30m", stale)
	}
	threshold := c.EffectiveSuggestionThreshold()
	if threshold != 5 {
		t.Errorf("custom suggestion threshold = %d, want 5", threshold)
	}
}

func TestStalenessConfigInvalidDuration(t *testing.T) {
	c := &config.StalenessConfig{
		FreshDuration: "not-a-duration",
		StaleDuration: "also-bad",
	}
	fresh, stale := c.ParsedStalenessThresholds()
	// Should fall back to defaults
	if fresh != 24*time.Hour {
		t.Errorf("invalid fresh = %v, want 24h (default)", fresh)
	}
	if stale != 48*time.Hour {
		t.Errorf("invalid stale = %v, want 48h (default)", stale)
	}
}
