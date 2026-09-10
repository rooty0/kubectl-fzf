package kubectlfzfserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDesiredContexts(t *testing.T) {
	testCases := []struct {
		name          string
		current       string
		previous      string
		watchPrevious bool
		expected      []string
	}{
		{"no current context", "", "", true, nil},
		{"current only, flag off", "minikube", "", false, []string{"minikube"}},
		{"current only, flag on", "minikube", "", true, []string{"minikube"}},
		{"first switch, flag off", "prod", "minikube", false, []string{"prod"}},
		{"first switch, flag on", "prod", "minikube", true, []string{"prod", "minikube"}},
		{"current equals previous", "prod", "prod", true, []string{"prod"}},
		{"flip flop keeps both", "minikube", "prod", true, []string{"minikube", "prod"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, desiredContexts(tc.current, tc.previous, tc.watchPrevious))
		})
	}
}
