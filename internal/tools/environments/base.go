package environments

// Environment defines the interface for command execution environments.
// Implementations include local execution, Docker containers, and SSH remote hosts.
type Environment interface {
	// Execute runs a command in the environment and returns its output.
	// timeout is in seconds. Returns stdout, stderr, exit code, and any error.
	Execute(command string, timeout int) (stdout, stderr string, exitCode int, err error)

	// IsAvailable checks if this environment is ready for use.
	IsAvailable() bool

	// Name returns the human-readable name of this environment.
	Name() string
}

// EnvironmentFactory creates an Environment from configuration parameters.
type EnvironmentFactory func(params map[string]string) (Environment, error)

// registry holds all registered environment factories.
var registry = map[string]EnvironmentFactory{}

// RegisterEnvironment registers an environment factory under a name.
func RegisterEnvironment(name string, factory EnvironmentFactory) {
	registry[name] = factory
}

// GetEnvironment creates an environment by name with the given parameters.
func GetEnvironment(name string, params map[string]string) (Environment, error) {
	factory, ok := registry[name]
	if !ok {
		// Default to local
		return NewLocalEnvironment(), nil
	}
	return factory(params)
}

// ListEnvironments returns the names of all registered environments.
func ListEnvironments() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
