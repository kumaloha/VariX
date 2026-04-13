package normalize

import "regexp"

var (
	leadingSpeaker = regexp.MustCompile(`^([^\s：:|\-【】]{2,6})[：:|\-]\s*\S`)
	bracketSpeaker = regexp.MustCompile(`【([^】]{2,6})】`)
	interviewName  = regexp.MustCompile(`(?:专访|对谈|访谈|采访|连线)\s*([^\s，,、。！？\-|:：]{2,6})`)
	featureName    = regexp.MustCompile(`\bft\.?\s+([A-Z][a-zA-Z]+(?: [A-Z][a-zA-Z]+){0,2})`)
	chineseName    = regexp.MustCompile(`^[\p{Han}]{2,4}$`)
	englishName    = regexp.MustCompile(`^[A-Z][a-zA-Z]+(?: [A-Z][a-zA-Z]+)*$`)
)

func ExtractSpeakerFromTitle(title string) string {
	for _, pattern := range []*regexp.Regexp{leadingSpeaker, bracketSpeaker, interviewName, featureName} {
		match := pattern.FindStringSubmatch(title)
		if len(match) < 2 {
			continue
		}
		candidate := match[1]
		if looksLikeName(candidate) {
			return candidate
		}
	}
	return ""
}

func looksLikeName(value string) bool {
	return chineseName.MatchString(value) || englishName.MatchString(value)
}
