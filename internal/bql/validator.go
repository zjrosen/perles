package bql

import (
	"fmt"
	"strings"
)

// ValidFields defines the set of valid field names in BQL.
var ValidFields = map[string]FieldType{
	"type":     FieldEnum,
	"priority": FieldPriority,
	"status":   FieldEnum,
	"blocked":  FieldBool,
	"ready":    FieldBool,
	"label":    FieldString,
	"labels":   FieldString,
	"title":    FieldString,
	"id":       FieldString,
	"created":  FieldDate,
	"updated":  FieldDate,
}

// FieldType categorizes fields for validation.
type FieldType int

const (
	FieldString FieldType = iota
	FieldEnum
	FieldPriority
	FieldBool
	FieldDate
)

// ValidTypeValues are the valid values for the type field.
var ValidTypeValues = map[string]bool{
	"bug":     true,
	"feature": true,
	"task":    true,
	"epic":    true,
	"chore":   true,
}

// ValidStatusValues are the valid values for the status field.
var ValidStatusValues = map[string]bool{
	"open":        true,
	"in_progress": true,
	"closed":      true,
	"blocked":     true,
}

// ValidPriorityValues are the valid values for the priority field.
var ValidPriorityValues = map[string]bool{
	"P0": true, "p0": true,
	"P1": true, "p1": true,
	"P2": true, "p2": true,
	"P3": true, "p3": true,
	"P4": true, "p4": true,
}

// Validate validates a BQL query and returns an error if invalid.
func Validate(query *Query) error {
	if query.Filter != nil {
		if err := validateExpr(query.Filter); err != nil {
			return err
		}
	}

	for _, term := range query.OrderBy {
		if err := validateOrderField(term.Field); err != nil {
			return err
		}
	}

	return nil
}

// validateExpr validates an expression recursively.
func validateExpr(expr Expr) error {
	switch e := expr.(type) {
	case *BinaryExpr:
		if err := validateExpr(e.Left); err != nil {
			return err
		}
		return validateExpr(e.Right)

	case *NotExpr:
		return validateExpr(e.Expr)

	case *CompareExpr:
		return validateCompare(e)

	case *InExpr:
		return validateIn(e)
	}

	return nil
}

// validateCompare validates a comparison expression.
func validateCompare(e *CompareExpr) error {
	// Check field exists
	fieldType, ok := ValidFields[e.Field]
	if !ok {
		return fmt.Errorf("unknown field: %q (valid: %s)", e.Field, validFieldNames())
	}

	// Check operator is valid for field type
	if err := validateOperator(e.Field, fieldType, e.Op); err != nil {
		return err
	}

	// Check value is valid for field type
	return validateValue(e.Field, fieldType, e.Value)
}

// validateIn validates an IN expression.
func validateIn(e *InExpr) error {
	// Check field exists
	fieldType, ok := ValidFields[e.Field]
	if !ok {
		return fmt.Errorf("unknown field: %q (valid: %s)", e.Field, validFieldNames())
	}

	// IN is only valid for enum, string, and priority fields
	if fieldType == FieldBool || fieldType == FieldDate {
		return fmt.Errorf("operator IN is not valid for field %q", e.Field)
	}

	// Validate each value
	for _, v := range e.Values {
		if err := validateValue(e.Field, fieldType, v); err != nil {
			return err
		}
	}

	return nil
}

// validateOperator checks if an operator is valid for a field type.
func validateOperator(field string, fieldType FieldType, op TokenType) error {
	switch fieldType {
	case FieldBool:
		// Boolean fields only support = and !=
		if op != TokenEq && op != TokenNeq {
			return fmt.Errorf("operator %q is not valid for boolean field %q (use = or !=)", op, field)
		}

	case FieldEnum:
		// Enum fields support = and !=
		if op != TokenEq && op != TokenNeq {
			return fmt.Errorf("operator %q is not valid for field %q (use = or !=)", op, field)
		}

	case FieldString:
		// String fields support =, !=, ~, !~
		if op != TokenEq && op != TokenNeq && op != TokenContains && op != TokenNotContains {
			return fmt.Errorf("operator %q is not valid for string field %q (use =, !=, ~, or !~)", op, field)
		}

	case FieldPriority:
		// Priority supports all comparison operators
		// (already validated by parser)

	case FieldDate:
		// Date supports comparison operators, but not ~
		if op == TokenContains || op == TokenNotContains {
			return fmt.Errorf("operator %q is not valid for date field %q", op, field)
		}
	}

	return nil
}

// validateValue checks if a value is valid for a field type.
func validateValue(field string, fieldType FieldType, value Value) error {
	switch fieldType {
	case FieldBool:
		if value.Type != ValueBool {
			return fmt.Errorf("field %q requires a boolean value (true or false)", field)
		}

	case FieldPriority:
		if value.Type != ValuePriority {
			return fmt.Errorf("field %q requires a priority value (P0-P4), got %q", field, value.Raw)
		}

	case FieldDate:
		if value.Type != ValueDate {
			return fmt.Errorf("field %q requires a date value (today, yesterday, -Nd, or ISO date), got %q", field, value.Raw)
		}

	case FieldEnum:
		// Validate enum values
		switch field {
		case "type":
			if !ValidTypeValues[value.String] {
				return fmt.Errorf("invalid value %q for field %q (valid: bug, feature, task, epic, chore)", value.String, field)
			}
		case "status":
			if !ValidStatusValues[value.String] {
				return fmt.Errorf("invalid value %q for field %q (valid: open, in_progress, closed, blocked)", value.String, field)
			}
		}

	case FieldString:
		// Any string value is valid
	}

	return nil
}

// validateOrderField checks if a field can be used in ORDER BY.
func validateOrderField(field string) error {
	// Check field exists
	_, ok := ValidFields[field]
	if !ok {
		return fmt.Errorf("unknown field in ORDER BY: %q (valid: %s)", field, validFieldNames())
	}
	return nil
}

// validFieldNames returns a comma-separated list of valid field names.
func validFieldNames() string {
	names := make([]string, 0, len(ValidFields))
	for name := range ValidFields {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}
