package host

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type OSRelease struct {
	ID         string `json:"id"`
	VersionID  string `json:"versionId"`
	Codename   string `json:"codename,omitempty"`
	PrettyName string `json:"prettyName,omitempty"`
}

func LoadOSRelease(path string) (OSRelease, error) {
	file, err := os.Open(path)
	if err != nil {
		return OSRelease{}, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		values[key] = parseOSReleaseValue(value)
	}
	if err := scanner.Err(); err != nil {
		return OSRelease{}, err
	}

	return OSRelease{
		ID:         values["ID"],
		VersionID:  values["VERSION_ID"],
		Codename:   firstNonEmpty(values["UBUNTU_CODENAME"], values["VERSION_CODENAME"]),
		PrettyName: values["PRETTY_NAME"],
	}, nil
}

func ValidateSupportedUbuntu(release OSRelease) error {
	if release.ID != "ubuntu" {
		return fmt.Errorf("unsupported operating system %q: Ubuntu is required", release.ID)
	}
	if release.VersionID != "22.04" && release.VersionID != "24.04" {
		return fmt.Errorf("unsupported Ubuntu version %q: supported versions are 22.04 and 24.04", release.VersionID)
	}
	return nil
}

func parseOSReleaseValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	return strings.Trim(value, "'")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
