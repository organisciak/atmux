package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "atmux.conf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestParseRemoteHostDirectives(t *testing.T) {
	path := writeTempConfig(t, `
remote_host:user@devbox.example.com
remote_alias:devbox
remote_port:2201
remote_attach:mosh

remote_host:user@buildbox.example.com
`)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if got, want := len(cfg.RemoteHosts), 2; got != want {
		t.Fatalf("expected %d remote hosts, got %d", want, got)
	}

	first := cfg.RemoteHosts[0]
	if first.Host != "user@devbox.example.com" {
		t.Fatalf("first host mismatch: %q", first.Host)
	}
	if first.Alias != "devbox" {
		t.Fatalf("first alias mismatch: %q", first.Alias)
	}
	if first.Port != 2201 {
		t.Fatalf("first port mismatch: %d", first.Port)
	}
	if first.AttachMethod != "mosh" {
		t.Fatalf("first attach method mismatch: %q", first.AttachMethod)
	}

	second := cfg.RemoteHosts[1]
	if second.Host != "user@buildbox.example.com" {
		t.Fatalf("second host mismatch: %q", second.Host)
	}
	if second.Alias != "user@buildbox.example.com" {
		t.Fatalf("second alias mismatch: %q", second.Alias)
	}
	if second.Port != 22 {
		t.Fatalf("second port mismatch: %d", second.Port)
	}
	if second.AttachMethod != "ssh" {
		t.Fatalf("second attach method mismatch: %q", second.AttachMethod)
	}
}

func TestParseRemoteDirectiveRequiresRemoteHost(t *testing.T) {
	testCases := []struct {
		name      string
		content   string
		wantError string
	}{
		{
			name: "alias without host",
			content: `
remote_alias:devbox
`,
			wantError: "remote_alias requires a preceding remote_host",
		},
		{
			name: "port without host",
			content: `
remote_port:2201
`,
			wantError: "remote_port requires a preceding remote_host",
		},
		{
			name: "attach without host",
			content: `
remote_attach:mosh
`,
			wantError: "remote_attach requires a preceding remote_host",
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

func TestParseRemoteDirectiveInvalidValues(t *testing.T) {
	testCases := []struct {
		name      string
		content   string
		wantError string
	}{
		{
			name: "invalid port",
			content: `
remote_host:user@devbox.example.com
remote_port:not-a-number
`,
			wantError: "invalid remote_port",
		},
		{
			name: "invalid attach method",
			content: `
remote_host:user@devbox.example.com
remote_attach:telnet
`,
			wantError: "remote_attach must be 'ssh' or 'mosh'",
		},
		{
			name: "empty host",
			content: `
remote_host:
`,
			wantError: "remote_host requires a host value",
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

func TestMergeConfigsRemoteHostsLocalOverridesByAlias(t *testing.T) {
	global := &Config{
		RemoteHosts: []RemoteHostConfig{
			{Host: "user@dev-global", Alias: "dev", Port: 22, AttachMethod: "ssh"},
			{Host: "user@db", Alias: "db", Port: 22, AttachMethod: "ssh"},
		},
	}
	local := &Config{
		RemoteHosts: []RemoteHostConfig{
			{Host: "user@dev-local", Alias: "dev", Port: 2202, AttachMethod: "mosh"},
			{Host: "user@new", Alias: "new", Port: 22, AttachMethod: "ssh"},
		},
	}

	merged := mergeConfigs(global, local)
	if got, want := len(merged.RemoteHosts), 3; got != want {
		t.Fatalf("expected %d remote hosts, got %d", want, got)
	}

	first := merged.RemoteHosts[0]
	if first.Host != "user@dev-local" || first.Alias != "dev" || first.Port != 2202 || first.AttachMethod != "mosh" {
		t.Fatalf("unexpected first merged host: %+v", first)
	}

	second := merged.RemoteHosts[1]
	if second.Host != "user@db" || second.Alias != "db" {
		t.Fatalf("unexpected second merged host: %+v", second)
	}

	third := merged.RemoteHosts[2]
	if third.Host != "user@new" || third.Alias != "new" {
		t.Fatalf("unexpected third merged host: %+v", third)
	}
}

func TestResolveRemoteHosts(t *testing.T) {
	cfg := &Config{
		RemoteHosts: []RemoteHostConfig{
			{Host: "user@devbox.example.com", Alias: "devbox", Port: 2201, AttachMethod: "mosh"},
			{Host: "user@prod.example.com", Alias: "prod", Port: 22, AttachMethod: "ssh"},
		},
	}

	t.Run("resolve aliases hosts and ad-hoc values", func(t *testing.T) {
		hosts, err := ResolveRemoteHosts(cfg, "devbox,user@prod.example.com,newhost", false)
		if err != nil {
			t.Fatalf("ResolveRemoteHosts returned error: %v", err)
		}
		if got, want := len(hosts), 3; got != want {
			t.Fatalf("expected %d hosts, got %d", want, got)
		}

		if hosts[0].Host != "user@devbox.example.com" || hosts[0].Port != 2201 || hosts[0].AttachMethod != "mosh" {
			t.Fatalf("unexpected first resolved host: %+v", hosts[0])
		}
		if hosts[1].Host != "user@prod.example.com" || hosts[1].Alias != "prod" {
			t.Fatalf("unexpected second resolved host: %+v", hosts[1])
		}
		if hosts[2].Host != "newhost" || hosts[2].Alias != "newhost" || hosts[2].Port != 22 || hosts[2].AttachMethod != "ssh" {
			t.Fatalf("unexpected third resolved host: %+v", hosts[2])
		}
	})

	t.Run("dedupe same host via alias and host token", func(t *testing.T) {
		hosts, err := ResolveRemoteHosts(cfg, "devbox,user@devbox.example.com", false)
		if err != nil {
			t.Fatalf("ResolveRemoteHosts returned error: %v", err)
		}
		if got, want := len(hosts), 1; got != want {
			t.Fatalf("expected %d host, got %d", want, got)
		}
	})

	t.Run("include configured when flag empty", func(t *testing.T) {
		hosts, err := ResolveRemoteHosts(cfg, "", true)
		if err != nil {
			t.Fatalf("ResolveRemoteHosts returned error: %v", err)
		}
		if got, want := len(hosts), 2; got != want {
			t.Fatalf("expected %d hosts, got %d", want, got)
		}
	})
}
