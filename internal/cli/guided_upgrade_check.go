package cli

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const upgradeChangelogBase = "https://raw.githubusercontent.com/Chemaclass/agnostic-ai/"

func stableRelease(version string) bool {
	r, err := config.ParseRequirement(version)
	if err != nil {
		return false
	}
	allowed, stable := r.Allows(version)
	return allowed && stable
}

func newerStableRelease(latest, current string) bool {
	if !stableRelease(latest) || !stableRelease(current) {
		return false
	}
	r, err := config.ParseRequirement("<" + strings.TrimPrefix(latest, "v"))
	if err != nil {
		return false
	}
	allowed, _ := r.Allows(current)
	return allowed
}

func fetchUpgradeOffer(current string) (upgradeOffer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	latest, err := fetchLatestRelease(2 * time.Second)
	if err != nil {
		return upgradeOffer{}, err
	}
	latest = strings.TrimPrefix(latest, "v")
	if !newerStableRelease(latest, current) {
		return upgradeOffer{Latest: latest}, nil
	}
	offer := upgradeOffer{Latest: latest}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	offer.Guidance, offer.GuidanceError = fetchReleaseDocs(ctx, client, current, latest)
	if offer.GuidanceError != "" {
		url := upgradeChangelogBase + "v" + latest + "/CHANGELOG.md"
		body, err := readUpgradeDoc(ctx, client, url, 2<<20)
		if err == nil {
			fallback, err := releaseGuidance(string(body), current, latest)
			if err == nil {
				offer.Guidance += "\nChangelog fallback (upgrade docs unavailable for some releases):\n" + fallback
			}
		}
	}

	return offer, nil
}

var changelogReleaseHeading = regexp.MustCompile(`^## (?:v|\[v?)([0-9]+\.[0-9]+\.[0-9]+)(?:\]|\s|$)`)

func releaseGuidance(changelog, current, latest string) (string, error) {
	var selected strings.Builder
	include, foundLatest := false, false
	for _, line := range strings.Split(changelog, "\n") {
		if strings.HasPrefix(line, "## ") {
			include = false
			if match := changelogReleaseHeading.FindStringSubmatch(line); len(match) > 1 {
				version := match[1]
				include = newerStableRelease(version, current) && !newerStableRelease(version, latest)
				if include && versionsEqual(version, latest) {
					foundLatest = true
				}
			}
		}
		if include {
			selected.WriteString(line)
			selected.WriteByte('\n')
		}
	}
	if !foundLatest {
		return "", fmt.Errorf("the changelog has no section for %s", latest)
	}
	clean := cleanUpgradeGuidance(selected.String())
	return clean, nil
}

func cleanUpgradeGuidance(text string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
	if len(clean) > 64<<10 {
		clean = clean[:64<<10] + "\nFurther guidance is on the release pages.\n"
	}
	return clean
}
