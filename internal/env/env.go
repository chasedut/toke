package env

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Env interface {
	Get(key string) string
	Env() []string
}

type osEnv struct{}

// Get implements Env.
func (o *osEnv) Get(key string) string {
	return os.Getenv(key)
}

func (o *osEnv) Env() []string {
	env := os.Environ()
	if len(env) == 0 {
		return nil
	}
	return env
}

func New() Env {
	return &osEnv{}
}

type mapEnv struct {
	m map[string]string
}

// Get implements Env.
func (m *mapEnv) Get(key string) string {
	if value, ok := m.m[key]; ok {
		return value
	}
	return ""
}

// Env implements Env.
func (m *mapEnv) Env() []string {
	if len(m.m) == 0 {
		return nil
	}
	env := make([]string, 0, len(m.m))
	for k, v := range m.m {
		env = append(env, k+"="+v)
	}
	return env
}

func NewFromMap(m map[string]string) Env {
	if m == nil {
		m = make(map[string]string)
	}
	return &mapEnv{m: m}
}

// LoadDotEnv loads environment variables from a .env file
func LoadDotEnv() error {
	// Look for .env file in current directory first
	envPath := ".env"
	
	// If not found, try to find it relative to executable
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		exePath, err := os.Executable()
		if err == nil {
			exeDir := filepath.Dir(exePath)
			altPath := filepath.Join(exeDir, ".env")
			if _, err := os.Stat(altPath); err == nil {
				envPath = altPath
			}
		}
	}
	
	// If still not found, return nil (not an error to not have .env)
	file, err := os.Open(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Split on first = sign
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		
		// Remove quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		
		// Only set if not already set (environment takes precedence)
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
	
	return scanner.Err()
}
