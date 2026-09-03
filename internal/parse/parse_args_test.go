package parse

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseContextFromArgs(t *testing.T) {
	testDatas := []struct {
		name     string
		args     []string
		expected *string
	}{
		{"the separated form", []string{"pods", "--context", "prod"}, ptr("prod")},
		{"the attached form", []string{"pods", "--context=prod"}, ptr("prod")},
		{"named before the resource", []string{"--context", "prod", "pods", " "}, ptr("prod")},
		{"no context named", []string{"pods", "-n", "kube-system"}, nil},
		// The value is missing, and the flag behind it is not a context name.
		{"a flag where the value would be", []string{"pods", "--context", "-o", "yaml"}, nil},
		{"nothing behind the flag at all", []string{"pods", "--context"}, nil},
		{"a word only being started", []string{"pods", "--context", " "}, nil},
		// Past the separator the arguments belong to the executed command.
		{"named past the separator", []string{"exec", "pod", "--", "prog", "--context", "x"}, nil},
	}
	for _, testData := range testDatas {
		t.Run(testData.name, func(t *testing.T) {
			got := ParseContextFromArgs(testData.args)
			if testData.expected == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, *testData.expected, *got)
		})
	}
}

func ptr(s string) *string { return &s }

func TestWordsBesideCursor(t *testing.T) {
	args := []string{"pods", "--context", "pro", "-n", "kube-system"}

	// A context half typed under the cursor names nothing yet.
	assert.Equal(t, []string{"pods", "--context", "-n", "kube-system"}, WordsBesideCursor(args, 2))
	assert.Equal(t, []string{"--context", "pro", "-n", "kube-system"}, WordsBesideCursor(args, 0))
	assert.Equal(t, args, WordsBesideCursor(args, -1))
	assert.Equal(t, args, WordsBesideCursor(args, 9))
}

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
