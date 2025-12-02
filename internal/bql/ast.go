package bql

// Node is the interface for all AST nodes.
type Node interface {
	node()
}

// Expr is the interface for expression nodes.
type Expr interface {
	Node
	expr()
}

// Query represents a complete BQL query.
type Query struct {
	Filter  Expr        // The filter expression (may be nil for ORDER BY only queries)
	OrderBy []OrderTerm // ORDER BY terms (may be empty)
}

func (q *Query) node() {}

// BinaryExpr represents "expr AND/OR expr".
type BinaryExpr struct {
	Left  Expr
	Op    TokenType // TokenAnd or TokenOr
	Right Expr
}

func (b *BinaryExpr) node() {}
func (b *BinaryExpr) expr() {}

// NotExpr represents "NOT expr".
type NotExpr struct {
	Expr Expr
}

func (n *NotExpr) node() {}
func (n *NotExpr) expr() {}

// CompareExpr represents "field op value".
type CompareExpr struct {
	Field string
	Op    TokenType
	Value Value
}

func (c *CompareExpr) node() {}
func (c *CompareExpr) expr() {}

// InExpr represents "field IN (values)" or "field NOT IN (values)".
type InExpr struct {
	Field  string
	Values []Value
	Not    bool // true for "NOT IN"
}

func (i *InExpr) node() {}
func (i *InExpr) expr() {}

// ValueType indicates the type of a Value.
type ValueType int

const (
	ValueString ValueType = iota
	ValueInt
	ValueBool
	ValuePriority // P0, P1, P2, P3, P4
	ValueDate     // today, yesterday, -7d, ISO dates
)

// Value represents a literal value in a query.
type Value struct {
	Type   ValueType
	Raw    string // Original string representation
	String string // String value (for ValueString, ValuePriority, ValueDate)
	Int    int    // Integer value (for ValueInt, also priority level for ValuePriority)
	Bool   bool   // Boolean value (for ValueBool)
}

// OrderTerm represents a single ORDER BY term.
type OrderTerm struct {
	Field string
	Desc  bool // true for DESC, false for ASC (default)
}
