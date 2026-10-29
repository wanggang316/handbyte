package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

func lstatGit(root string) (bool, error) {
	_, err := os.Lstat(filepath.Join(root, ".git"))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func gitEnvironment() []string {
	var env []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			env = append(env, value)
		}
	}
	return append(env, "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
}
