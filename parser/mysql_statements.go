// Package parser - mysql_statements.go
// MySQL-specific statement forms: VALUES ROW(), TABLE stmt, parenthesized queries.

package parser

import (
	"github.com/unoflavora/gomysqlx/ast"
	"github.com/unoflavora/gomysqlx/models"
)

// parseValuesStatement parses MySQL 8.0.19+ VALUES ROW(1, 'a'), ROW(2, 'b')
// Returns a synthetic SelectStatement for compatibility with the AST.
func (p *Parser) parseValuesStatement() (ast.Statement, error) {
	// VALUES already consumed. Parse ROW(...) expressions.
	stmt := &ast.SelectStatement{}

	// Skip all ROW(...) expressions — we don't need their values for permission checking
	for {
		if p.isTokenMatch("ROW") {
			p.advance() // Consume ROW
		}
		if p.isType(models.TokenTypeLParen) {
			depth := 1
			p.advance() // Consume (
			for depth > 0 && !p.isType(models.TokenTypeEOF) {
				if p.isType(models.TokenTypeLParen) {
					depth++
				} else if p.isType(models.TokenTypeRParen) {
					depth--
				}
				if depth > 0 {
					p.advance()
				}
			}
			if p.isType(models.TokenTypeRParen) {
				p.advance() // Consume )
			}
		}
		if !p.isType(models.TokenTypeComma) {
			break
		}
		p.advance() // Consume comma
	}

	return stmt, nil
}

// parseTableStatement parses MySQL 8.0.19+ TABLE stmt
// TABLE users ≡ SELECT * FROM users
func (p *Parser) parseTableStatement() (ast.Statement, error) {
	// TABLE already consumed. Next token is the table name.
	tableName, err := p.parseQualifiedName()
	if err != nil {
		return nil, p.expectedError("table name after TABLE")
	}

	return &ast.SelectStatement{
		From: []ast.TableReference{{Name: tableName}},
	}, nil
}

// parseParenthesizedQuery parses (SELECT ...) [ORDER BY] [LIMIT] or
// (SELECT ...) UNION ALL (SELECT ...) etc.
func (p *Parser) parseParenthesizedQuery() (ast.Statement, error) {
	// Current token is LPAREN
	p.advance() // Consume (

	// Parse the inner SELECT
	if !p.isType(models.TokenTypeSelect) && !p.isType(models.TokenTypeWith) {
		return nil, p.expectedError("SELECT inside parenthesized query")
	}

	p.advance() // Consume SELECT/WITH
	stmt, err := p.parseSelectWithSetOperations()
	if err != nil {
		return nil, err
	}

	// Expect closing paren
	if !p.isType(models.TokenTypeRParen) {
		return nil, p.expectedError(")")
	}
	p.advance() // Consume )

	// Check for set operations: UNION, INTERSECT, EXCEPT
	if p.isAnyType(models.TokenTypeUnion, models.TokenTypeExcept, models.TokenTypeIntersect) {
		opType := p.currentToken.Token.Value
		p.advance() // Consume UNION/EXCEPT/INTERSECT

		allFlag := false
		if p.isType(models.TokenTypeAll) {
			allFlag = true
			p.advance()
		} else if p.isType(models.TokenTypeDistinct) {
			p.advance()
		}

		// Parse right side
		var rightStmt ast.Statement
		if p.isType(models.TokenTypeLParen) {
			rightStmt, err = p.parseParenthesizedQuery()
		} else if p.isType(models.TokenTypeSelect) {
			p.advance()
			rightStmt, err = p.parseSelectWithSetOperations()
		}
		if err != nil {
			return nil, err
		}

		return &ast.SetOperation{
			Operator: opType,
			All:      allFlag,
			Left:     stmt,
			Right:    rightStmt,
		}, nil
	}

	// Check for ORDER BY / LIMIT after the parenthesized query
	if p.isType(models.TokenTypeOrder) {
		p.advance() // ORDER
		if p.isType(models.TokenTypeBy) {
			p.advance() // BY
			for {
				_, exprErr := p.parseExpression()
				if exprErr != nil {
					return nil, exprErr
				}
				if p.isTokenMatch("ASC") || p.isTokenMatch("DESC") {
					p.advance()
				}
				if !p.isType(models.TokenTypeComma) {
					break
				}
				p.advance()
			}
		}
	}

	if p.isType(models.TokenTypeLimit) {
		p.advance() // LIMIT
		if p.isNumericLiteral() {
			p.advance()
		}
	}

	return stmt, nil
}
