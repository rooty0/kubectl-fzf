package parse

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllNamespacesFlags(t *testing.T) {
	testDatas := []struct {
		name     string
		args     []string
		expected []string
	}{
		{"no flag", []string{"pods", " "}, nil},
		{"short", []string{"pods", "-A", " "}, []string{"-A"}},
		{"long", []string{"pods", "--all-namespaces", " "}, []string{"--all-namespaces"}},
		{"long with true", []string{"pods", "--all-namespaces=true", " "}, []string{"--all-namespaces=true"}},
		{"long with 1", []string{"pods", "--all-namespaces=1", " "}, []string{"--all-namespaces=1"}},
		{"long with false stays namespace scoped", []string{"pods", "--all-namespaces=false", " "}, nil},
		{"short with false stays namespace scoped", []string{"pods", "-A=false", " "}, nil},
		// Everything past -- is handed to the executed command, kubectl never
		// sees it, so it must not be mistaken for kubectl's own -A.
		{"after double dash belongs to the command", []string{"mypod", "--", "prog", "-A"}, nil},
		{"before and after double dash", []string{"-A", "mypod", "--", "prog", "-A"}, []string{"-A"}},
		// A value that merely contains -A is not the flag.
		{"flag value looking like the flag", []string{"pods", "-l", "app=-A", " "}, nil},
	}
	for _, testData := range testDatas {
		t.Run(testData.name, func(t *testing.T) {
			assert.Equal(t, testData.expected, AllNamespacesFlags(testData.args))
			assert.Equal(t, len(testData.expected) > 0, HasAllNamespacesFlag(testData.args))
		})
	}
}
