package auth

import (
	"os"

	"gopkg.in/yaml.v2"
)

// GetToken tries to infer the access token
// from environment variables and config files.
func GetToken() string {
	var token string

	// gh-tools specific env variable.
	if token = os.Getenv("GHTOOLS_TOKEN"); token != "" {
		return token
	}
	// Generic env variable.
	if token = os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	// Read the token from gh-tools auth file ~/.config/gh-tools/auth.yml
	if token = fromAuthFile(); token != "" {
		return token
	}
	// Try to read the token from gh cli's config file ~/.config/gh/hosts.yml
	if token = fromGhCliConfig(); token != "" {
		return token
	}

	return ""
}

func fromAuthFile() string {
	path := "/.config/gh-tools/auth.yml"

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path = home + path

	file, err := os.Open(path)
	if err != nil {
		return ""
	}

	auth := map[string]string{}
	err = yaml.NewDecoder(file).Decode(auth)
	if err != nil {
		return ""
	}

	return auth["oauth_token"]
}

func fromGhCliConfig() string {
	path := "/.config/gh/hosts.yml"

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path = home + path

	file, err := os.Open(path)
	if err != nil {
		return ""
	}

	hosts := map[string]struct {
		OauthToken string `yaml:"oauth_token"`
		User       string `yaml:"user"`
	}{}
	err = yaml.NewDecoder(file).Decode(hosts)
	if err != nil {
		return ""
	}

	auth := hosts["github.com"]

	return auth.OauthToken
}
