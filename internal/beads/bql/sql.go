package bql

import (
	"fmt"
	"strings"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
)

// jsonMetadataPath returns the JSON path for a metadata key. Like bd (beads
// internal/storage/metadata.go JSONMetadataPath), the key is always quoted, so
// a dotted key such as "jira.sprint" addresses the literal top-level key
// $."jira.sprint" rather than a nested {"jira": {"sprint": ...}} path. Both
// MySQL/Dolt and SQLite accept quoted path labels. Backslashes and quotes are
// escaped as defense in depth; the validator already rejects them.
func jsonMetadataPath(key string) string {
	return `$."` + jsonPathEscaper.Replace(key) + `"`
}

var jsonPathEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

const (
	legacyDependencyTargetColumn   = "depends_on_id"
	issueDependencyTargetColumn    = "depends_on_issue_id"
	externalDependencyTargetColumn = "depends_on_external"
)

func defaultDependencyTargetColumn(dialect appbeads.SQLDialect) string {
	if dialect == appbeads.DialectMySQL {
		return issueDependencyTargetColumn
	}
	return legacyDependencyTargetColumn
}

// ReadyExcludedTypes are the issue types bd leaves out of ready work (beads
// internal/storage/sqlbuild/ready.go ReadyWorkExcludeTypes): merge-request,
// gate, molecule and rig, plus bd's infra types (domain.DefaultInfraTypes:
// agent, role, message).
var ReadyExcludedTypes = []string{"merge-request", "gate", "molecule", "rig", "agent", "role", "message"}

// fieldColumns maps BQL fields whose name differs from their issues column.
var fieldColumns = map[string]string{
	"type":    "issue_type",
	"created": "created_at",
	"updated": "updated_at",
	"started": "started_at",
	"closed":  "closed_at",
	"due":     "due_at",
}

// nullableStringFields are string fields wrapped in COALESCE with an empty
// string, so that a NULL column behaves like an empty string.
var nullableStringFields = map[string]bool{
	"assignee":            true,
	"owner":               true,
	"close_reason":        true,
	"acceptance_criteria": true,
}

// issueColumnForField returns the issues column a BQL field reads, or false
// for fields that are computed (blocked, ready) or live in another table
// (label). Metadata fields read the metadata column.
func issueColumnForField(field string) (string, bool) {
	switch field {
	case "blocked", "ready", "label":
		return "", false
	}
	if strings.HasPrefix(field, "metadata.") {
		return "metadata", true
	}
	if col, ok := fieldColumns[field]; ok {
		return col, true
	}
	return field, true
}

// Schema describes the optional parts of the beads schema that BQL SQL can
// reference. Real beads databases span many schema versions, so the executor
// probes the database once and builds SQL for what it finds.
type Schema struct {
	// DependencyTarget is the dependencies column holding a local target
	// issue ID: depends_on_issue_id (beads migration 0041+) or the legacy
	// depends_on_id. Empty means the dialect default.
	DependencyTarget string
	// HasDependsOnExternal reports dependencies.depends_on_external, where
	// beads stores external: refs and every edge whose target has a different
	// ID prefix than its source (beads issueops.IsExternalDepTarget).
	HasDependsOnExternal bool
	// HasIsBlocked reports the maintained issues.is_blocked flag (beads
	// migration 0046+), bd's source of truth for blocked work.
	HasIsBlocked bool
	// HasCustomStatuses reports the custom_statuses(name, category) table.
	HasCustomStatuses bool
	// HasCustomTypes reports the custom_types(name) table.
	HasCustomTypes bool
	// IssueColumns is the set of columns of the issues table. Nil means
	// unknown, in which case every column is assumed to exist.
	IssueColumns map[string]bool
}

// HasIssueColumn reports whether the issues table has the column. It returns
// true when the column set is unknown.
func (s *Schema) HasIssueColumn(column string) bool {
	if s == nil || s.IssueColumns == nil {
		return true
	}
	return s.IssueColumns[column]
}

// DependencyTargetExpr returns the SQL expression for a dependency row's
// target issue ID under the given dependencies table alias. When the
// depends_on_external column exists, cross-prefix targets are stored there,
// so the target is COALESCE(<target column>, depends_on_external). Callers
// join the expression to issues so only local issues match.
func (s *Schema) DependencyTargetExpr(alias string) string {
	column := alias + "." + s.DependencyTarget
	if s.HasDependsOnExternal {
		return "COALESCE(" + column + ", " + alias + "." + externalDependencyTargetColumn + ")"
	}
	return column
}

// SQLBuilder converts a BQL AST to SQL.
type SQLBuilder struct {
	query      *Query
	params     []any
	dialect    appbeads.SQLDialect
	readySQL   func(isReady bool) string   // optional override for ready field SQL
	blockedSQL func(isBlocked bool) string // optional override for blocked field SQL
	schema     Schema
}

// SQLBuilderOption configures a SQLBuilder.
type SQLBuilderOption func(*SQLBuilder)

// WithReadySQL overrides the default ready field SQL generation.
// This is used by backends that lack a ready_issues view (e.g. beads_rust)
// to provide an inline subquery instead.
func WithReadySQL(fn func(isReady bool) string) SQLBuilderOption {
	return func(b *SQLBuilder) { b.readySQL = fn }
}

// WithBlockedSQL overrides the default blocked field SQL generation.
func WithBlockedSQL(fn func(isBlocked bool) string) SQLBuilderOption {
	return func(b *SQLBuilder) { b.blockedSQL = fn }
}

// WithDependencyTargetColumn overrides the dependencies table target column.
func WithDependencyTargetColumn(column string) SQLBuilderOption {
	return func(b *SQLBuilder) { b.schema.DependencyTarget = column }
}

// WithSchema builds SQL for a probed schema. Without it the builder assumes
// the dialect's default dependency target column, no is_blocked column, no
// depends_on_external column, no custom_statuses table, and that every issues
// column exists.
func WithSchema(schema *Schema) SQLBuilderOption {
	return func(b *SQLBuilder) {
		if schema != nil {
			b.schema = *schema
		}
	}
}

// NewSQLBuilder creates a builder for the query and dialect.
func NewSQLBuilder(query *Query, dialect appbeads.SQLDialect, opts ...SQLBuilderOption) *SQLBuilder {
	b := &SQLBuilder{query: query, dialect: dialect}
	for _, opt := range opts {
		opt(b)
	}
	if b.schema.DependencyTarget == "" {
		b.schema.DependencyTarget = defaultDependencyTargetColumn(dialect)
	}
	return b
}

// Build generates the SQL WHERE clause and ORDER BY.
func (b *SQLBuilder) Build() (whereClause string, orderBy string, params []any) {
	if b.query.Filter != nil {
		whereClause = b.buildExpr(b.query.Filter)
	}

	if len(b.query.OrderBy) > 0 {
		orderBy = b.buildOrderBy()
	}

	return whereClause, orderBy, b.params
}

// buildExpr recursively builds SQL for an expression.
func (b *SQLBuilder) buildExpr(expr Expr) string {
	switch e := expr.(type) {
	case *BinaryExpr:
		left := b.buildExpr(e.Left)
		right := b.buildExpr(e.Right)
		op := "AND"
		if e.Op == TokenOr {
			op = "OR"
		}
		return fmt.Sprintf("(%s %s %s)", left, op, right)

	case *NotExpr:
		return fmt.Sprintf("NOT (%s)", b.buildExpr(e.Expr))

	case *CompareExpr:
		return b.buildCompare(e)

	case *InExpr:
		return b.buildIn(e)
	}

	return ""
}

// buildCompare builds SQL for a comparison expression.
func (b *SQLBuilder) buildCompare(e *CompareExpr) string {
	// Handle special fields
	switch e.Field {
	case "blocked":
		return b.buildBlockedSQL(e.Value.Bool)

	case "ready":
		return b.buildReadySQL(e.Value.Bool)

	case "pinned", "is_template":
		// Nullable boolean columns (INTEGER in SQLite)
		// field = true -> column = 1
		// field = false -> column = 0 (won't match NULL)
		column := b.fieldToColumn(e.Field)
		if e.Value.Bool {
			return fmt.Sprintf("%s = 1", column)
		}
		return fmt.Sprintf("%s = 0", column)

	case "label":
		// Label check via labels table
		// Supports exact match (=, !=) and partial match (~, !~)
		switch e.Op {
		case TokenContains:
			return "i.id IN (SELECT issue_id FROM labels WHERE " + b.likeSQL("label", TokenContains, e.Value.String) + ")"
		case TokenNotContains:
			return "i.id NOT IN (SELECT issue_id FROM labels WHERE " + b.likeSQL("label", TokenContains, e.Value.String) + ")"
		case TokenNeq:
			b.params = append(b.params, e.Value.String)
			return "i.id NOT IN (SELECT issue_id FROM labels WHERE label = ?)"
		default: // TokenEq
			b.params = append(b.params, e.Value.String)
			return "i.id IN (SELECT issue_id FROM labels WHERE label = ?)"
		}
	}

	if key, ok := strings.CutPrefix(e.Field, "metadata."); ok {
		path := jsonMetadataPath(key)

		if e.Value.Type == ValueNull {
			return b.metadataKeySQL(path, e.Op == TokenNeq)
		}

		extracted := b.metadataValueSQL(path)
		switch e.Op {
		case TokenContains, TokenNotContains:
			return b.likeSQL(extracted, e.Op, e.Value.String)
		default:
			b.params = append(b.params, e.Value.String)
			return fmt.Sprintf("%s %s ?", extracted, b.opToSQL(e.Op))
		}
	}

	// Map BQL fields to SQL columns
	column := b.fieldToColumn(e.Field)

	// Handle priority comparisons
	if e.Field == "priority" {
		b.params = append(b.params, e.Value.Int)
		return fmt.Sprintf("%s %s ?", column, b.opToSQL(e.Op))
	}

	// Handle date comparisons
	if e.Value.Type == ValueDate {
		dateSQL := b.dateToSQL(e.Value.String)
		if b.dialect == appbeads.DialectMySQL {
			// MySQL/Dolt handles datetime types natively, no wrapping needed
			return fmt.Sprintf("%s %s %s", column, b.opToSQL(e.Op), dateSQL)
		}
		// SQLite: wrap column in datetime() to normalize ISO 8601 timestamps
		// with timezone to UTC format that matches datetime('now', ...) expressions
		return fmt.Sprintf("datetime(%s) %s %s", column, b.opToSQL(e.Op), dateSQL)
	}

	// Handle nullable string fields (use COALESCE so NULL matches empty string)
	if nullableStringFields[e.Field] {
		switch e.Op {
		case TokenContains, TokenNotContains:
			return b.likeSQL(fmt.Sprintf("COALESCE(%s, '')", column), e.Op, e.Value.String)
		default:
			b.params = append(b.params, e.Value.String)
			return fmt.Sprintf("COALESCE(%s, '') %s ?", column, b.opToSQL(e.Op))
		}
	}

	// Handle contains/not contains operators
	switch e.Op {
	case TokenContains, TokenNotContains:
		return b.likeSQL(column, e.Op, e.Value.String)
	}

	// Standard comparison
	b.params = append(b.params, e.Value.String)
	return fmt.Sprintf("%s %s ?", column, b.opToSQL(e.Op))
}

// buildIn builds SQL for an IN expression.
func (b *SQLBuilder) buildIn(e *InExpr) string {
	// Handle label field specially
	if e.Field == "label" {
		placeholders := make([]string, len(e.Values))
		for i, v := range e.Values {
			placeholders[i] = "?"
			b.params = append(b.params, v.String)
		}
		subquery := fmt.Sprintf("i.id IN (SELECT issue_id FROM labels WHERE label IN (%s))",
			strings.Join(placeholders, ", "))
		if e.Not {
			return "NOT " + subquery
		}
		return subquery
	}

	column := b.fieldToColumn(e.Field)
	placeholders := make([]string, len(e.Values))

	for i, v := range e.Values {
		placeholders[i] = "?"
		if e.Field == "priority" {
			b.params = append(b.params, v.Int)
		} else {
			b.params = append(b.params, v.String)
		}
	}

	op := "IN"
	if e.Not {
		op = "NOT IN"
	}

	return fmt.Sprintf("%s %s (%s)", column, op, strings.Join(placeholders, ", "))
}

// fieldToColumn maps BQL field names to qualified SQL column names.
func (b *SQLBuilder) fieldToColumn(field string) string {
	if col, ok := fieldColumns[field]; ok {
		return "i." + col
	}
	return "i." + field
}

// metadataKeySQL builds the metadata key existence check for "= nil" and
// "!= nil" and appends the path param. A key that holds JSON null counts as
// present on both dialects: MySQL/Dolt's JSON_EXTRACT returns a JSON null
// (not SQL NULL) for it, and SQLite's json_type returns 'null', whereas
// SQLite's json_extract would return SQL NULL.
func (b *SQLBuilder) metadataKeySQL(path string, exists bool) string {
	b.params = append(b.params, path)
	expr := "JSON_TYPE(i.metadata, ?)"
	if b.dialect == appbeads.DialectMySQL {
		expr = "JSON_EXTRACT(i.metadata, ?)"
	}
	if exists {
		return expr + " IS NOT NULL"
	}
	return expr + " IS NULL"
}

// metadataValueSQL returns a text expression for the metadata value at path
// and appends the path params it references. On MySQL/Dolt this is
// JSON_UNQUOTE(JSON_EXTRACT(...)): strings are unquoted, every other JSON value
// (numbers, true/false, null, objects, arrays) becomes its JSON text, and a
// missing key yields NULL. SQLite has no JSON_UNQUOTE, and its json_extract
// returns INTEGER/REAL for numbers and 1/0 for booleans, which never equal a
// string param, so there only text values are read with json_extract and
// everything else is read as JSON text with the -> operator.
func (b *SQLBuilder) metadataValueSQL(path string) string {
	if b.dialect == appbeads.DialectMySQL {
		b.params = append(b.params, path)
		return "JSON_UNQUOTE(JSON_EXTRACT(i.metadata, ?))"
	}
	b.params = append(b.params, path, path, path)
	return "CASE JSON_TYPE(i.metadata, ?) WHEN 'text' THEN JSON_EXTRACT(i.metadata, ?) ELSE i.metadata -> ? END"
}

// likeSQL builds a case-insensitive substring match for the ~ and !~ operators
// and appends the pattern param. SQLite's LIKE is already case-insensitive for
// ASCII, but MySQL/Dolt's default utf8mb4_0900_bin collation is case-sensitive,
// so both sides are lowercased there.
func (b *SQLBuilder) likeSQL(expr string, op TokenType, value string) string {
	pattern := "%" + value + "%"
	if b.dialect == appbeads.DialectMySQL {
		expr = "LOWER(" + expr + ")"
		pattern = strings.ToLower(pattern)
	}
	b.params = append(b.params, pattern)
	if op == TokenNotContains {
		return expr + " NOT LIKE ?"
	}
	return expr + " LIKE ?"
}

// opToSQL converts a token operator to SQL.
func (b *SQLBuilder) opToSQL(op TokenType) string {
	switch op {
	case TokenEq:
		return "="
	case TokenNeq:
		return "!="
	case TokenLt:
		return "<"
	case TokenGt:
		return ">"
	case TokenLte:
		return "<="
	case TokenGte:
		return ">="
	default:
		return "="
	}
}

// dateToSQL converts a date value to a SQL expression based on the dialect.
func (b *SQLBuilder) dateToSQL(dateStr string) string {
	if b.dialect == appbeads.DialectMySQL {
		return b.dateToSQLMySQL(dateStr)
	}
	return b.dateToSQLSQLite(dateStr)
}

// dateToSQLSQLite generates SQLite date expressions. SQLite's 'now' is UTC.
func (b *SQLBuilder) dateToSQLSQLite(dateStr string) string {
	switch dateStr {
	case "now":
		return "datetime('now')"
	case "today":
		return "date('now')"
	case "yesterday":
		return "date('now', '-1 day')"
	default:
		// Handle relative time formats: -Nd (days), -Nh (hours), -Nm (months)
		if len(dateStr) > 1 && dateStr[0] == '-' {
			suffix := dateStr[len(dateStr)-1]
			value := dateStr[1 : len(dateStr)-1] // strip - and suffix

			switch suffix {
			case 'd', 'D':
				return fmt.Sprintf("date('now', '-%s days')", value)
			case 'h', 'H':
				// Hours use datetime() for sub-day precision
				return fmt.Sprintf("datetime('now', '-%s hours')", value)
			case 'm', 'M':
				return fmt.Sprintf("date('now', '-%s months')", value)
			}
		}
		// Assume ISO date, pass through as string
		b.params = append(b.params, dateStr)
		return "?"
	}
}

// mysqlNow is the current time on MySQL/Dolt. bd stores DATETIME columns in
// UTC and compares them with UTC_TIMESTAMP() (e.g. defer_until in bd ready),
// so BQL uses UTC too instead of the session-time-zone NOW()/CURDATE(). This
// also matches SQLite, whose datetime('now') is UTC. Dolt has no UTC_DATE(),
// so dates are DATE(UTC_TIMESTAMP()).
const (
	mysqlNow   = "UTC_TIMESTAMP()"
	mysqlToday = "DATE(" + mysqlNow + ")"
)

// dateToSQLMySQL generates MySQL/Dolt date expressions.
func (b *SQLBuilder) dateToSQLMySQL(dateStr string) string {
	switch dateStr {
	case "now":
		return mysqlNow
	case "today":
		return mysqlToday
	case "yesterday":
		return "DATE_SUB(" + mysqlToday + ", INTERVAL 1 DAY)"
	default:
		// Handle relative time formats: -Nd (days), -Nh (hours), -Nm (months)
		if len(dateStr) > 1 && dateStr[0] == '-' {
			suffix := dateStr[len(dateStr)-1]
			value := dateStr[1 : len(dateStr)-1] // strip - and suffix

			switch suffix {
			case 'd', 'D':
				return fmt.Sprintf("DATE_SUB(%s, INTERVAL %s DAY)", mysqlToday, value)
			case 'h', 'H':
				return fmt.Sprintf("DATE_SUB(%s, INTERVAL %s HOUR)", mysqlNow, value)
			case 'm', 'M':
				return fmt.Sprintf("DATE_SUB(%s, INTERVAL %s MONTH)", mysqlToday, value)
			}
		}
		// Assume ISO date, pass through as string
		b.params = append(b.params, dateStr)
		return "?"
	}
}

// doneFrozenStatusesSubquery selects custom statuses that, like closed and
// pinned, neither block nor are blocked (beads migration 0045 blocked_issues).
const doneFrozenStatusesSubquery = "SELECT name FROM custom_statuses WHERE category IN ('done', 'frozen')"

// activeStatusesSubquery selects custom statuses that count as open for ready
// work (beads ready_issues view since migration 0025, bd ready).
const activeStatusesSubquery = "SELECT name FROM custom_statuses WHERE category = 'active'"

// blockingStatusExpr is true when an issue's status takes part in blocking:
// anything but closed, pinned and custom done/frozen statuses.
func (b *SQLBuilder) blockingStatusExpr(alias string) string {
	expr := alias + ".status NOT IN ('closed', 'pinned')"
	if b.schema.HasCustomStatuses {
		expr += " AND " + alias + ".status NOT IN (" + doneFrozenStatusesSubquery + ")"
	}
	return expr
}

// isBlockedFlagExpr mirrors bd blocked (beads issueops GetBlockedIssuesInTx):
// the maintained is_blocked flag, excluding closed and pinned issues.
func isBlockedFlagExpr(alias string) string {
	return fmt.Sprintf("(%s.is_blocked = 1 AND %s.status NOT IN ('closed', 'pinned'))", alias, alias)
}

// doltBlockedSubquery returns the inlined SQL for finding blocked issues in
// Dolt databases without issues.is_blocked. It mirrors the blocked_issues view
// (beads migration 0045) and is inlined to bypass a Dolt server bug where
// views return stale field index errors after client reconnection.
func (b *SQLBuilder) doltBlockedSubquery() string {
	return `SELECT bi.id FROM issues bi
WHERE ` + b.blockingStatusExpr("bi") + `
AND EXISTS (
  SELECT 1 FROM dependencies d
  WHERE d.issue_id = bi.id AND d.type = 'blocks'
  AND EXISTS (
    SELECT 1 FROM issues blocker
    WHERE blocker.id = ` + b.schema.DependencyTargetExpr("d") + `
    AND ` + b.blockingStatusExpr("blocker") + `
  )
)`
}

// blockedIDsSubquery selects blocked issue IDs for databases without
// issues.is_blocked: the inlined blocked_issues view on Dolt, and
// blocked_issues_cache on SQLite.
func (b *SQLBuilder) blockedIDsSubquery() string {
	if b.dialect == appbeads.DialectMySQL {
		return b.doltBlockedSubquery()
	}
	return "SELECT issue_id FROM blocked_issues_cache"
}

// buildBlockedSQL returns the SQL fragment for the blocked field. When the
// issues table has is_blocked it is used directly, as bd blocked does;
// otherwise Dolt inlines the blocked_issues view and SQLite reads
// blocked_issues_cache. Backends can override via WithBlockedSQL.
func (b *SQLBuilder) buildBlockedSQL(isBlocked bool) string {
	if b.blockedSQL != nil {
		return b.blockedSQL(isBlocked)
	}
	if b.schema.HasIsBlocked {
		if isBlocked {
			return isBlockedFlagExpr("i")
		}
		return "NOT " + isBlockedFlagExpr("i")
	}
	if isBlocked {
		return "i.id IN (" + b.blockedIDsSubquery() + ")"
	}
	return "i.id NOT IN (" + b.blockedIDsSubquery() + ")"
}

// buildReadySQL returns the SQL fragment for the ready field, which mirrors
// bd ready (beads sqlbuild.BuildReadyWorkWhere). It is inlined instead of
// using the ready_issues view because older views lack bd's current
// semantics. Backends can override via WithReadySQL.
func (b *SQLBuilder) buildReadySQL(isReady bool) string {
	if b.readySQL != nil {
		return b.readySQL(isReady)
	}

	readyExpr := "(" + b.readyExpression("i") + ")"
	if isReady {
		return readyExpr
	}
	return "NOT " + readyExpr
}

// readyExpression returns the AND-joined ready predicates for the issue alias:
// open (or a custom status with category active), not pinned, not ephemeral,
// not one of bd's excluded types, not deferred into the future, not blocked,
// and no parent deferred into the future.
func (b *SQLBuilder) readyExpression(alias string) string {
	status := alias + ".status = 'open'"
	if b.schema.HasCustomStatuses {
		status = "(" + status + " OR " + alias + ".status IN (" + activeStatusesSubquery + "))"
	}
	parts := []string{status}

	if b.schema.HasIssueColumn("pinned") {
		parts = append(parts, fmt.Sprintf("(%s.pinned = 0 OR %s.pinned IS NULL)", alias, alias))
	}
	if b.schema.HasIssueColumn("ephemeral") {
		parts = append(parts, fmt.Sprintf("(%s.ephemeral = 0 OR %s.ephemeral IS NULL)", alias, alias))
	}
	parts = append(parts, fmt.Sprintf("%s.issue_type NOT IN (%s)", alias, quotedSQLList(ReadyExcludedTypes)))
	parts = append(parts, b.notFutureDeferredExpression(alias))

	if b.schema.HasIsBlocked {
		// bd ready reads the maintained flag directly.
		parts = append(parts, alias+".is_blocked = 0")
	} else {
		parts = append(parts, fmt.Sprintf("%s.id NOT IN (%s)", alias, b.blockedIDsSubquery()))
	}

	parts = append(parts, b.noFutureDeferredParentExpression(alias))
	return strings.Join(parts, " AND ")
}

// notFutureDeferredExpression is true when the issue is not deferred into the
// future.
func (b *SQLBuilder) notFutureDeferredExpression(alias string) string {
	if b.dialect == appbeads.DialectMySQL {
		return fmt.Sprintf("(%s.defer_until IS NULL OR %s.defer_until <= %s)", alias, alias, mysqlNow)
	}
	return fmt.Sprintf("(%s.defer_until IS NULL OR datetime(%s.defer_until) <= datetime('now'))", alias, alias)
}

// noFutureDeferredParentExpression is true when no parent-child parent of the
// issue is deferred into the future (beads issueops deferredChildrenQuery).
func (b *SQLBuilder) noFutureDeferredParentExpression(alias string) string {
	future := "parent.defer_until > " + mysqlNow
	if b.dialect != appbeads.DialectMySQL {
		future = "datetime(parent.defer_until) > datetime('now')"
	}
	return fmt.Sprintf(`NOT EXISTS (
  SELECT 1 FROM dependencies dp
  JOIN issues parent ON parent.id = %s
  WHERE dp.issue_id = %s.id AND dp.type = 'parent-child'
  AND parent.defer_until IS NOT NULL AND %s
)`, b.schema.DependencyTargetExpr("dp"), alias, future)
}

// quotedSQLList renders hardcoded string constants as a SQL literal list.
func quotedSQLList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	return strings.Join(quoted, ", ")
}

// buildOrderBy builds the ORDER BY clause.
func (b *SQLBuilder) buildOrderBy() string {
	var parts []string
	for _, term := range b.query.OrderBy {
		col := b.fieldToColumn(term.Field)
		dir := "ASC"
		if term.Desc {
			dir = "DESC"
		}
		parts = append(parts, fmt.Sprintf("%s %s", col, dir))
	}
	return strings.Join(parts, ", ")
}
