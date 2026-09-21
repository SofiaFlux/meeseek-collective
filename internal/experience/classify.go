package experience

import (
	"strings"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
)

func genericTaskClassForScope(scope string) domain.GenericTaskClass {
	normalized := strings.ToLower(strings.TrimSpace(scope))
	tokens := strings.FieldsFunc(normalized, func(r rune) bool {
		switch r {
		case '.', '/', '_', '-', ':':
			return true
		default:
			return false
		}
	})
	for _, token := range tokens {
		switch token {
		case "debug", "debugging":
			return domain.GenericTaskDebugging
		case "review":
			return domain.GenericTaskReview
		case "refactor", "refactoring":
			return domain.GenericTaskRefactor
		case "doc", "docs", "documentation":
			return domain.GenericTaskDocumentation
		case "migrate", "migration":
			return domain.GenericTaskMigration
		case "test", "tests", "testing":
			return domain.GenericTaskTesting
		case "maint", "maintenance":
			return domain.GenericTaskMaintenance
		}
	}
	return domain.GenericTaskOther
}
