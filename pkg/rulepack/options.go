package rulepack

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

func optionNames(id string) []string {
	switch id {
	case "markdown.formatting":
		return []string{"allow-hard-breaks"}
	case "markdown.trailing-whitespace":
		return []string{"allow-hard-breaks"}
	case "markdown.ordered-list":
		return []string{"style"}
	case "markdown.line-length":
		return []string{"max"}
	case "markdown.image-alt":
		return []string{"allow"}
	case "markdown.list-style":
		return []string{"allow", "indent"}
	case "markdown.html-policy":
		return []string{"allow"}
	case "markdown.single-title":
		return []string{"frontmatter-title"}
	case "text.terminology":
		return []string{"terms", "allow", "scope"}
	case "text.spelling":
		return []string{"dictionary", "language", "allow", "scope"}
	case "text.repeated-word":
		return []string{"allow", "scope"}
	case "text.no-dashes":
		return []string{"scope"}
	case "text.no-load-bearing":
		return []string{"scope"}
	case "markdown.local-links":
		return []string{"allow-directories"}
	case "markdown.required-heading":
		return []string{"heading", "level"}
	case "markdown.document-identifier":
		return []string{"identifiers"}
	case "markdown.document-structure":
		return []string{"types"}
	case "markdown.link-relocation":
		return []string{"moves"}
	}
	return nil
}
func validateCheckOptions(id string, n yaml.Node) error {
	if n.Kind == 0 {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("options must be a mapping")
	}
	allowed := map[string]bool{}
	for _, key := range optionNames(id) {
		allowed[key] = true
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if !allowed[n.Content[i].Value] {
			return fmt.Errorf("check %s has no option %q", id, n.Content[i].Value)
		}
	}
	return nil
}
