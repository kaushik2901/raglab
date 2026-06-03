package preprocessor

import (
	"regexp"
	"strings"
)

type ShortcodeAction int

const (
	Remove ShortcodeAction = iota
	StripTags
	Resolve
)

type ShortcodeRule struct {
	Name   string
	Action ShortcodeAction
}

var shortcodeTagRe = regexp.MustCompile(`\{\{([<%])\s*(/?)(\w+)`)

func StripShortcodes(content string, rules []ShortcodeRule) string {
	ruleMap := make(map[string]ShortcodeAction)
	for _, r := range rules {
		ruleMap[r.Name] = r.Action
	}

	var result strings.Builder
	i := 0

	for i < len(content) {
		loc := shortcodeTagRe.FindStringSubmatchIndex(content[i:])
		if loc == nil {
			result.WriteString(content[i:])
			break
		}

		offset := i + loc[0]
		mode := content[i+loc[2] : i+loc[3]]
		isClosing := content[i+loc[4]:i+loc[5]] == "/"
		tagName := content[i+loc[6] : i+loc[7]]

		result.WriteString(content[i:offset])

		if isClosing {
			i = offset + (loc[1] - loc[0])
			continue
		}

		openEnd := findTagEnd(content, offset, mode)
		if openEnd < 0 {
			i = offset + (loc[1] - loc[0])
			continue
		}

		action, known := ruleMap[tagName]
		if !known {
			action = StripTags
		}

		closeStart := findMatchingClose(content, openEnd+1, tagName, mode)

		switch action {
		case Remove:
			if closeStart >= 0 {
				closeEnd := findTagEnd(content, closeStart, mode)
				if closeEnd >= 0 {
					i = closeEnd + 1
				} else {
					i = openEnd + 1
				}
			} else {
				i = openEnd + 1
			}
		case StripTags, Resolve:
			if closeStart >= 0 {
				result.WriteString(content[openEnd+1 : closeStart])
				closeEnd := findTagEnd(content, closeStart, mode)
				if closeEnd >= 0 {
					i = closeEnd + 1
				} else {
					i = openEnd + 1
				}
			} else {
				i = openEnd + 1
			}
		default:
			i = openEnd + 1
		}
	}

	return result.String()
}

func findTagEnd(content string, pos int, mode string) int {
	var delim string
	if mode == "<" {
		delim = ">}}"
	} else {
		delim = "%}}"
	}
	idx := strings.Index(content[pos:], delim)
	if idx < 0 {
		return -1
	}
	return pos + idx + len(delim) - 1
}

func findMatchingClose(content string, pos int, name string, mode string) int {
	closePattern := "{{" + mode + " /" + name
	idx := strings.Index(content[pos:], closePattern)
	if idx < 0 {
		altPattern := "{{/" + name
		idx = strings.Index(content[pos:], altPattern)
	}
	if idx < 0 {
		return -1
	}
	return pos + idx
}

func parseShortcodeName(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexAny(s, " \t\n"); idx >= 0 {
		return s[:idx]
	}
	return s
}

func defaultShortcodeRules() []ShortcodeRule {
	return []ShortcodeRule{
		{Name: "include", Action: Resolve},
		{Name: "details", Action: StripTags},
		{Name: "alert", Action: StripTags},
		{Name: "panel", Action: StripTags},
		{Name: "handbook-data-toc", Action: Remove},
		{Name: "youtube", Action: Remove},
		{Name: "member-by-name", Action: Remove},
		{Name: "member-by-gitlab", Action: Remove},
		{Name: "ref", Action: Resolve},
		{Name: "relref", Action: Resolve},
	}
}
