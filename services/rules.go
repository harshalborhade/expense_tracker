package services

import (
	"fmt"
	"regexp"

	"expense_tracker/database"

	"gorm.io/gorm"
)

type RuleEngine struct {
	DB    *gorm.DB
	Rules []CompiledRule
}

type CompiledRule struct {
	Regex    *regexp.Regexp
	Category string
}

func NewRuleEngine(db *gorm.DB) *RuleEngine {
	re := &RuleEngine{DB: db}
	re.Reload()
	return re
}

// Reload fetches rules from DB and compiles regexes for performance
func (re *RuleEngine) Reload() {
	var dbRules []database.CategoryRule
	// Sort by Priority DESC so specific rules override general ones
	re.DB.Order("priority desc").Find(&dbRules)

	var compiled []CompiledRule
	for _, r := range dbRules {
		// regex.MustCompile panics on bad regex, so we use Compile and log errors
		regex, err := regexp.Compile(r.Pattern)
		if err != nil {
			fmt.Printf("[WARN] Invalid Regex Rule '%s': %v\n", r.Pattern, err)
			continue
		}
		compiled = append(compiled, CompiledRule{
			Regex:    regex,
			Category: r.Category,
		})
	}
	re.Rules = compiled
	fmt.Printf("[INFO] Loaded %d auto-categorization rules\n", len(re.Rules))
}

// Apply returns the matched category or empty string
func (re *RuleEngine) Apply(payee string) string {
	for _, rule := range re.Rules {
		if rule.Regex.MatchString(payee) {
			return rule.Category
		}
	}
	return ""
}
