package remote

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProfileStoreLifecycle(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "hosts.yaml")}
	profile := Profile{Address: "server.example.com", User: "deploy", Port: 2222, Sudo: true}
	if err := store.Put("production", profile); err != nil {
		t.Fatal(err)
	}
	loaded, exists, err := store.Get("production")
	if err != nil || !exists || !reflect.DeepEqual(loaded, profile) {
		t.Fatalf("unexpected profile: %#v %v %v", loaded, exists, err)
	}
	removed, err := store.Remove("production")
	if err != nil || !removed {
		t.Fatalf("remove failed: %v %v", removed, err)
	}
}

func TestSSHArgumentsQuoteRemoteArguments(t *testing.T) {
	arguments := SSHArguments(Profile{Address: "example.com", User: "deploy", Port: 22, Sudo: true}, []string{"project", "show", "/srv/My Project", "--env", "production"}, false)
	command := arguments[len(arguments)-1]
	if !strings.Contains(command, `'sudo' '-n' 'omurga'`) || !strings.Contains(command, `'/srv/My Project'`) {
		t.Fatalf("unexpected remote command: %s", command)
	}
}

func TestRemoveHostFlag(t *testing.T) {
	actual := RemoveHostFlag([]string{"--host", "production", "--json", "doctor", "--host=ignored"})
	expected := []string{"--json", "doctor"}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("unexpected arguments: %#v", actual)
	}
}
