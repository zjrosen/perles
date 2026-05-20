package bql

import (
	"fmt"
	"regexp"
	"strings"
)

var validMetadataKeyRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)

func isMetadataField(field string) (bool, string) {
	if !strings.HasPrefix(field, "metadata.") {
		return false, ""
	}
	key := strings.TrimPrefix(field, "metadata.")
	return true, key
}

func validateMetadataKey(key string) error {
	if !validMetadataKeyRe.MatchString(key) {
		return fmt.Errorf("invalid metadata key %q: must match [a-zA-Z_][a-zA-Z0-9_.]*", key)
	}
	return nil
}

// ValidFields defines the set of valid field names in BQL.
var ValidFields = map[string]FieldType{
	"type":        FieldEnum,
	"priority":    FieldPriority,
	"status":      FieldEnum,
	"blocked":     FieldBool,
	"ready":       FieldBool,
	"pinned":      FieldBool,
	"is_template": FieldBool,
	"label":       FieldString,
	"title":       FieldString,
	"id":          FieldString,
	"assignee":    FieldString,
	"sender":      FieldString,
	"description": FieldString,
	"design":      FieldString,
	"notes":       FieldString,
	"created_by":  FieldString,
	"mol_type":    FieldString,
	"created":     FieldDate,
	"updated":     FieldDate,
	"defer_until": FieldDate,
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
	"bug":       true,
	"feature":   true,
	"task":      true,
	"epic":      true,
	"chore":     true,
	"milestone": true,
	"story":     true,
	"spike":     true,
}

// ValidPriorityValues are the valid values for the priority field.
var ValidPriorityValues = map[string]bool{
	"P0": true, "p0": true,
	"P1": true, "p1": true,
	"P2": true, "p2": true,
	"P3": true, "p3": true,
	"P4": true, "p4": true,
}

// Validate validates a BQL query against the default ValidFields.
func Validate(query *Query) error {
	return ValidateWithFields(query, ValidFields)
}

// ValidateWithFields validates a BQL query against a custom set of valid fields.
// This allows backends with different schemas (e.g. beads_rust) to reuse the BQL
// parser and SQL builder while validating against their own column set.
func ValidateWithFields(query *Query, validFields map[string]FieldType) error {
	if query.Filter != nil {
		if err := validateExpr(query.Filter, validFields); err != nil {
			return err
		}
	}

	for _, term := range query.OrderBy {
		if err := validateOrderField(term.Field, validFields); err != nil {
			return err
		}
	}

	return nil
}

// validateExpr validates an expression recursively.
func validateExpr(expr Expr, validFields map[string]FieldType) error {
	switch e := expr.(type) {
	case *BinaryExpr:
		if err := validateExpr(e.Left, validFields); err != nil {
			return err
		}
		return validateExpr(e.Right, validFields)

	case *NotExpr:
		return validateExpr(e.Expr, validFields)

	case *CompareExpr:
		return validateCompare(e, validFields)

	case *InExpr:
		return validateIn(e, validFields)
	}

	return nil
}

// validateCompare validates a comparison expression.
func validateCompare(e *CompareExpr, validFields map[string]FieldType) error {
	if isMeta, key := isMetadataField(e.Field); isMeta {
		if err := validateMetadataKey(key); err != nil {
			return err
		}
		if e.Op == TokenEq || e.Op == TokenNeq || e.Op == TokenContains || e.Op == TokenNotContains {
			if err := validateValue(e.Field, FieldString, e.Value); err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("operator %q is not valid for metadata field %q (use =, !=, ~, or !~)", e.Op, e.Field)
	}

	// Check field exists
	fieldType, ok := validFields[e.Field]
	if !ok {
		return fmt.Errorf("unknown field: %q (valid: %s)", e.Field, fieldNamesFromMap(validFields))
	}

	// Check operator is valid for field type
	if err := validateOperator(e.Field, fieldType, e.Op); err != nil {
		return err
	}

	// Check value is valid for field type
	return validateValue(e.Field, fieldType, e.Value)
}

// validateIn validates an IN expression.
func validateIn(e *InExpr, validFields map[string]FieldType) error {
	// Check field exists
	fieldType, ok := validFields[e.Field]
	if !ok {
		return fmt.Errorf("unknown field: %q (valid: %s)", e.Field, fieldNamesFromMap(validFields))
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
	if isMeta, _ := isMetadataField(field); isMeta {
		if value.Type == ValueNull || value.Type == ValueString {
			return nil
		}
		return fmt.Errorf("metadata field %q requires a string or nil value, got %q", field, value.Raw)
	}

	switch fieldType {
	case FieldBool:
		if value.Type != ValueBool {
			return fmt.Errorf("field %q requires a boolean value (true or false)", field)
		}

	case FieldPriority:
		// Accept both P0-P4 format and plain integers 0-4
		switch value.Type {
		case ValuePriority:
			// Already validated by parser
		case ValueInt:
			if value.Int < 0 || value.Int > 4 {
				return fmt.Errorf("field %q requires priority 0-4, got %d", field, value.Int)
			}
		default:
			return fmt.Errorf("field %q requires a priority value (P0-P4 or 0-4), got %q", field, value.Raw)
		}

	case FieldDate:
		if value.Type != ValueDate {
			return fmt.Errorf("field %q requires a date value (today, yesterday, -Nd, or ISO date), got %q", field, value.Raw)
		}

	case FieldEnum:
		// Validate enum values (status accepts any string to support custom statuses)
		switch field {
		case "type":
			if !ValidTypeValues[value.String] {
				return fmt.Errorf("invalid value %q for field %q (valid: bug, feature, task, epic, chore, milestone, story, spike)", value.String, field)
			}
		}

	case FieldString:
		// Any string value is valid
	}

	return nil
}

// validateOrderField checks if a field can be used in ORDER BY.
func validateOrderField(field string, validFields map[string]FieldType) error {
	// Check field exists
	_, ok := validFields[field]
	if !ok {
		return fmt.Errorf("unknown field in ORDER BY: %q (valid: %s)", field, fieldNamesFromMap(validFields))
	}
	return nil
}

// fieldNamesFromMap returns a comma-separated list of field names from the given map.
func fieldNamesFromMap(fields map[string]FieldType) string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}
