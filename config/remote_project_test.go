package config

import (
	"strings"
	"testing"
)

func TestParseRemoteProjectDirectives(t *testing.T) {
	path := writeTempConfig(t, `
remote_project:atmux
remote_project_host:devbox
remote_project_dir:/home/user/projects/atmux
remote_project_session:agent-atmux-main

remote_project:dotfiles
remote_project_host:user@shell.example.com
remote_project_dir:/home/user/dotfiles
`)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if got, want := len(cfg.RemoteProjects), 2; got != want {
		t.Fatalf("expected %d remote projects, got %d", want, got)
	}

	first := cfg.RemoteProjects[0]
	if first.Name != "atmux" || first.Host != "devbox" || first.WorkingDir != "/home/user/projects/atmux" || first.SessionName != "agent-atmux-main" {
		t.Fatalf("unexpected first remote project: %+v", first)
	}

	second := cfg.RemoteProjects[1]
	if second.Name != "dotfiles" || second.Host != "user@shell.example.com" || second.WorkingDir != "/home/user/dotfiles" {
		t.Fatalf("unexpected second remote project: %+v", second)
	}
	if second.SessionName != "agent-dotfiles" {
		t.Fatalf("expected default session agent-dotfiles, got %q", second.SessionName)
	}
}

func TestParseRemoteProjectDirectiveRequiresRemoteProject(t *testing.T) {
	testCases := []struct {
		name      string
		content   string
		wantError string
	}{
		{
			name: "host without project",
			content: `
remote_project_host:devbox
`,
			wantError: "remote_project_host requires a preceding remote_project",
		},
		{
			name: "dir without project",
			content: `
remote_project_dir:/tmp
`,
			wantError: "remote_project_dir requires a preceding remote_project",
		},
		{
			name: "session without project",
			content: `
remote_project_session:agent-atmux
`,
			wantError: "remote_project_session requires a preceding remote_project",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempConfig(t, tc.content)
			_, err := Parse(path)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got %q", tc.wantError, err.Error())
			}
		})
	}
}

func TestParseRemoteProjectDirectiveInvalidValues(t *testing.T) {
	testCases := []struct {
		name      string
		content   string
		wantError string
	}{
		{
			name: "empty name",
			content: `
remote_project:
`,
			wantError: "remote_project requires a name",
		},
		{
			name: "missing host",
			content: `
remote_project:atmux
remote_project_dir:/home/user/projects/atmux
`,
			wantError: "invalid remote project \"atmux\": host is required",
		},
		{
			name: "missing dir",
			content: `
remote_project:atmux
remote_project_host:devbox
`,
			wantError: "invalid remote project \"atmux\": working directory is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempConfig(t, tc.content)
			_, err := Parse(path)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got %q", tc.wantError, err.Error())
			}
		})
	}
}

func TestMergeConfigsRemoteProjectsLocalOverridesByName(t *testing.T) {
	global := &Config{
		RemoteProjects: []RemoteProjectConfig{
			{Name: "atmux", Host: "global-devbox", WorkingDir: "/srv/atmux", SessionName: "agent-atmux"},
			{Name: "dotfiles", Host: "global-shell", WorkingDir: "/srv/dotfiles", SessionName: "agent-dotfiles"},
		},
	}
	local := &Config{
		RemoteProjects: []RemoteProjectConfig{
			{Name: "atmux", Host: "local-devbox", WorkingDir: "/home/user/atmux", SessionName: "agent-atmux-local"},
			{Name: "infra", Host: "infra-host", WorkingDir: "/home/user/infra", SessionName: "agent-infra"},
		},
	}

	merged := mergeConfigs(global, local)
	if got, want := len(merged.RemoteProjects), 3; got != want {
		t.Fatalf("expected %d remote projects, got %d", want, got)
	}

	first := merged.RemoteProjects[0]
	if first.Name != "atmux" || first.Host != "local-devbox" || first.WorkingDir != "/home/user/atmux" {
		t.Fatalf("unexpected first merged remote project: %+v", first)
	}

	second := merged.RemoteProjects[1]
	if second.Name != "dotfiles" || second.Host != "global-shell" {
		t.Fatalf("unexpected second merged remote project: %+v", second)
	}

	third := merged.RemoteProjects[2]
	if third.Name != "infra" || third.Host != "infra-host" {
		t.Fatalf("unexpected third merged remote project: %+v", third)
	}
}

func TestAppendRemoteProject(t *testing.T) {
	path := writeTempConfig(t, `
# global config
remote_host:user@devbox.example.com
remote_alias:devbox
`)

	project := RemoteProjectConfig{
		Name:       "atmux",
		Host:       "devbox",
		WorkingDir: "/home/user/projects/atmux",
	}
	if err := AppendRemoteProject(path, project); err != nil {
		t.Fatalf("AppendRemoteProject returned error: %v", err)
	}

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse returned error after append: %v", err)
	}
	if got, want := len(cfg.RemoteProjects), 1; got != want {
		t.Fatalf("expected %d remote project after append, got %d", want, got)
	}

	got := cfg.RemoteProjects[0]
	if got.Name != "atmux" || got.Host != "devbox" || got.WorkingDir != "/home/user/projects/atmux" || got.SessionName != "agent-atmux" {
		t.Fatalf("unexpected appended remote project: %+v", got)
	}
}
