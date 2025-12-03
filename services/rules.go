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

func (re *RuleEngine) Reload() {
	var dbRules []database.CategoryRule
	re.DB.Order("priority desc").Find(&dbRules)

	var compiled []CompiledRule
	for _, r := range dbRules {
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

func (re *RuleEngine) Apply(payee string) string {
	for _, rule := range re.Rules {
		if rule.Regex.MatchString(payee) {
			return rule.Category
		}
	}
	return ""
}

// Run rules on all unreviewed transactions in the DB
func (re *RuleEngine) ApplyToExisting() (int, error) {
	var txs []database.Transaction

	// Only touch transactions that haven't been manually reviewed yet
	if err := re.DB.Where("is_reviewed = ?", false).Find(&txs).Error; err != nil {
		return 0, err
	}

	count := 0
	for _, tx := range txs {
		match := re.Apply(tx.Payee)

		// If we found a match, and it's different from the current category
		if match != "" && match != tx.LedgerCategory {
			tx.LedgerCategory = match
			// We DO NOT set IsReviewed=true here.
			// We want the user to still see them as "Pending" to verify the rule worked correctly.
			re.DB.Save(&tx)
			count++
		}
	}
	return count, nil
}
