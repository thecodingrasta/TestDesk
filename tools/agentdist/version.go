package main

import (
	"fmt"
	"regexp"
)

// Release versions have three forms. The external tag is v<VERSION>, so VERSION keeps the exact tag
// text. Plugin manifests and native bundles need numeric or semantic syntax, which these helpers derive:
//
//	release      manifest     bundle (Info.plist, Windows)
//	1.2.3        1.2.3        1.2.3
//	1.2.3-rc.1   1.2.3-rc.1   1.2.3
//	1.21a        1.21.1       1.21.1
//	1.21b        1.21.2       1.21.2
//
// A single letter is a patch revision: a is patch 1, b is patch 2, and so on through z. Only
// hyphenated semantic versions are prereleases. docs/RELEASING.md records the policy.
var (
	semverReleasePattern   = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$`)
	letteredReleasePattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)([a-z])$`)
)

func validDistributionVersion(value string) bool {
	return len(value) <= 64 && (semverReleasePattern.MatchString(value) || letteredReleasePattern.MatchString(value))
}

// letterPatch maps a single lowercase letter to its patch number: a is 1, z is 26.
func letterPatch(letter byte) int {
	return int(letter-'a') + 1
}

// manifestVersion returns the semantic version that plugin manifests must carry.
func manifestVersion(release string) (string, error) {
	if match := letteredReleasePattern.FindStringSubmatch(release); match != nil {
		return fmt.Sprintf("%s.%s.%d", match[1], match[2], letterPatch(match[3][0])), nil
	}
	if semverReleasePattern.MatchString(release) && len(release) <= 64 {
		return release, nil
	}
	return "", fmt.Errorf("release version %q cannot be mapped to a plugin manifest version", release)
}

// bundleVersion returns the numeric-only version that Info.plist and Windows version resources carry.
func bundleVersion(release string) (string, error) {
	if match := letteredReleasePattern.FindStringSubmatch(release); match != nil {
		return fmt.Sprintf("%s.%s.%d", match[1], match[2], letterPatch(match[3][0])), nil
	}
	if match := semverReleasePattern.FindStringSubmatch(release); match != nil && len(release) <= 64 {
		return fmt.Sprintf("%s.%s.%s", match[1], match[2], match[3]), nil
	}
	return "", fmt.Errorf("release version %q cannot be mapped to a bundle version", release)
}
