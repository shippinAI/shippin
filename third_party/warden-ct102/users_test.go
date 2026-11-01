package main

import (
	"testing"
)

func TestParsePasswdLine(t *testing.T) {
	tests := []struct {
		line     string
		expected *User
	}{
		{
			"root:x:0:0:root:/root:/bin/bash",
			&User{Username: "root", UID: 0, GID: 0, Comment: "root", Home: "/root", Shell: "/bin/bash"},
		},
		{
			"haggis:x:1000:1000:Haggis Tech:/home/haggis:/bin/bash",
			&User{Username: "haggis", UID: 1000, GID: 1000, Comment: "Haggis Tech", Home: "/home/haggis", Shell: "/bin/bash"},
		},
		{
			"nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin",
			&User{Username: "nobody", UID: 65534, GID: 65534, Comment: "nobody", Home: "/nonexistent", Shell: "/usr/sbin/nologin"},
		},
		{"short:x:1000", nil},         // too few fields
		{"", nil},                      // empty line
		{"a:b:c:d:e:f", nil},          // only 6 fields, need 7
	}

	for _, tt := range tests {
		result := parsePasswdLine(tt.line)
		if tt.expected == nil {
			if result != nil {
				t.Errorf("parsePasswdLine(%q) = %+v, want nil", tt.line, result)
			}
			continue
		}
		if result == nil {
			t.Errorf("parsePasswdLine(%q) = nil, want %+v", tt.line, tt.expected)
			continue
		}
		if result.Username != tt.expected.Username {
			t.Errorf("Username = %q, want %q", result.Username, tt.expected.Username)
		}
		if result.UID != tt.expected.UID {
			t.Errorf("UID = %d, want %d", result.UID, tt.expected.UID)
		}
		if result.GID != tt.expected.GID {
			t.Errorf("GID = %d, want %d", result.GID, tt.expected.GID)
		}
		if result.Home != tt.expected.Home {
			t.Errorf("Home = %q, want %q", result.Home, tt.expected.Home)
		}
		if result.Shell != tt.expected.Shell {
			t.Errorf("Shell = %q, want %q", result.Shell, tt.expected.Shell)
		}
	}
}

func TestParseGroupLine(t *testing.T) {
	tests := []struct {
		line        string
		expectNil   bool
		name        string
		gid         int
		memberCount int
	}{
		{"sudo:x:27:haggis,admin", false, "sudo", 27, 2},
		{"docker:x:999:haggis", false, "docker", 999, 1},
		{"nogroup:x:65534:", false, "nogroup", 65534, 0},
		{"root:x:0:", false, "root", 0, 0},
		{"short:x", true, "", 0, 0},
		{"", true, "", 0, 0},
	}

	for _, tt := range tests {
		result := parseGroupLine(tt.line)
		if tt.expectNil {
			if result != nil {
				t.Errorf("parseGroupLine(%q) = %+v, want nil", tt.line, result)
			}
			continue
		}
		if result == nil {
			t.Errorf("parseGroupLine(%q) = nil, want non-nil", tt.line)
			continue
		}
		if result.Name != tt.name {
			t.Errorf("Name = %q, want %q", result.Name, tt.name)
		}
		if result.GID != tt.gid {
			t.Errorf("GID = %d, want %d", result.GID, tt.gid)
		}
		if len(result.Members) != tt.memberCount {
			t.Errorf("Members count = %d, want %d", len(result.Members), tt.memberCount)
		}
	}
}

func TestParseGroups(t *testing.T) {
	input := "root:x:0:\nsudo:x:27:haggis\n# comment\n\ndocker:x:999:haggis,admin"
	groups := parseGroups(input)
	if len(groups) != 3 {
		t.Fatalf("parseGroups returned %d groups, want 3", len(groups))
	}
	// Should be sorted by GID
	if groups[0].Name != "root" {
		t.Errorf("first group = %q, want root", groups[0].Name)
	}
	if groups[1].Name != "sudo" {
		t.Errorf("second group = %q, want sudo", groups[1].Name)
	}
	if groups[2].Name != "docker" {
		t.Errorf("third group = %q, want docker", groups[2].Name)
	}
}

func TestValidUsername(t *testing.T) {
	valid := []string{"root", "haggis", "test_user", "_backup", "user-1", "a1b2c3"}
	invalid := []string{"Root", "123abc", "-bad", "user name", "user@host", "", "user.name"}

	for _, u := range valid {
		if !validUsername.MatchString(u) {
			t.Errorf("validUsername should accept %q", u)
		}
	}
	for _, u := range invalid {
		if validUsername.MatchString(u) {
			t.Errorf("validUsername should reject %q", u)
		}
	}
}
