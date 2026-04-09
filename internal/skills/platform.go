package skills

import "runtime"

// compilePlatform returns the GOOS value.
func compilePlatform() string {
	return runtime.GOOS
}
