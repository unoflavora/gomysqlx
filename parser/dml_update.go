// Copyright 2026 GoSQLX Authors
// Copyright 2026 gomysqlx Authors (MySQL extensions)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Package parser - dml_update.go
// UPDATE statement parsing with full MySQL support.
//
// MySQL syntax:
//
//	UPDATE [LOW_PRIORITY] [IGNORE] table_references
//	SET assignment_list
//	[WHERE where_condition]
//	[ORDER BY ...]
//	[LIMIT row_count]
//
// table_references can be:
//   - Single table: UPDATE users SET ...
//   - Aliased: UPDATE users u SET u.name = ...
//   - Comma join: UPDATE a, b SET a.x = b.y WHERE ...
//   - JOIN: UPDATE a JOIN b ON a.id = b.id SET a.x = b.y WHERE ...

package parser

import (
	"strings"

	"github.com/unoflavora/gomysqlx/ast"
	"github.com/unoflavora/gomysqlx/models"
)

// parseUpdateStatement parses a MySQL UPDATE statement.
func (p *Parser) parseUpdateStatement() (ast.Statement, error) {
	// We've already consumed the UPDATE token in matchType

	// Skip MySQL modifiers: LOW_PRIORITY, IGNORE
	for p.isTokenMatch("LOW_PRIORITY") || p.isTokenMatch("IGNORE") {
		p.advance()
	}

	// Parse first table reference (name + optional alias)
	firstRef, err := p.parseFromTableReference()
	if err != nil {
		return nil, p.expectedError("table name")
	}

	tableName := firstRef.Name
	tableAlias := firstRef.Alias
	var from []ast.TableReference

	// Check for comma-separated tables or JOIN clauses before SET
	var joins []ast.JoinClause

	// Comma-separated tables: UPDATE a, b SET ...
	for p.isType(models.TokenTypeComma) {
		p.advance()
		ref, err := p.parseFromTableReference()
		if err != nil {
			return nil, err
		}
		from = append(from, ref)
	}

	// JOIN clauses: UPDATE a JOIN b ON ... SET ...
	if p.isJoinKeyword() {
		joins, err = p.parseJoinClauses(firstRef)
		if err != nil {
			return nil, err
		}
		// Add joined tables to from list for table extraction
		for _, j := range joins {
			from = append(from, j.Right)
		}
	}

	// Parse SET
	if !p.isType(models.TokenTypeSet) {
		return nil, p.expectedError("SET")
	}
	p.advance() // Consume SET

	// Parse assignments
	updates := make([]ast.UpdateExpression, 0)
	for {
		// Parse column name — may be qualified: table.column or alias.column
		if !p.isIdentifier() && !p.isNonReservedKeyword() {
			return nil, p.expectedError("column name")
		}
		columnName := p.currentToken.Token.Value
		p.advance()

		// Handle qualified column: table.column
		if p.isType(models.TokenTypeDot) {
			p.advance() // Consume .
			if !p.isIdentifier() && !p.isNonReservedKeyword() {
				return nil, p.expectedError("column name after '.'")
			}
			columnName = columnName + "." + p.currentToken.Token.Value
			p.advance()
		}

		if !p.isType(models.TokenTypeEq) {
			return nil, p.expectedError("=")
		}
		p.advance() // Consume =

		// Parse value expression
		var expr ast.Expression
		if p.isStringLiteral() {
			expr = &ast.LiteralValue{Value: p.currentToken.Token.Value, Type: "string"}
			p.advance()
		} else if p.isNumericLiteral() {
			litType := "int"
			if strings.ContainsAny(p.currentToken.Token.Value, ".eE") {
				litType = "float"
			}
			expr = &ast.LiteralValue{Value: p.currentToken.Token.Value, Type: litType}
			p.advance()
		} else if p.isBooleanLiteral() {
			expr = &ast.LiteralValue{Value: p.currentToken.Token.Value, Type: "bool"}
			p.advance()
		} else {
			expr, err = p.parseExpression()
			if err != nil {
				return nil, err
			}
		}

		columnExpr := &ast.Identifier{Name: columnName}
		updates = append(updates, ast.UpdateExpression{
			Column: columnExpr,
			Value:  expr,
		})

		// More assignments?
		if !p.isType(models.TokenTypeComma) {
			break
		}
		p.advance() // Consume comma
	}

	// Parse WHERE clause if present
	var whereClause ast.Expression
	if p.isType(models.TokenTypeWhere) {
		p.advance() // Consume WHERE
		whereClause, err = p.parseExpression()
		if err != nil {
			return nil, err
		}
	}

	// Parse ORDER BY clause if present (MySQL)
	if p.isType(models.TokenTypeOrder) {
		p.advance() // Consume ORDER
		if p.isType(models.TokenTypeBy) {
			p.advance() // Consume BY
			// Skip order expressions — we don't need them in the AST for permission checking
			for {
				_, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				// ASC/DESC
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

	// Parse LIMIT clause if present (MySQL)
	if p.isType(models.TokenTypeLimit) {
		p.advance() // Consume LIMIT
		if p.isNumericLiteral() {
			p.advance() // Consume limit value
		}
	}

	// Parse RETURNING clause if present (PostgreSQL)
	var returning []ast.Expression
	if p.isType(models.TokenTypeReturning) || p.isTokenMatch("RETURNING") {
		p.advance() // Consume RETURNING
		returning, err = p.parseReturningColumns()
		if err != nil {
			return nil, err
		}
	}

	return &ast.UpdateStatement{
		TableName:   tableName,
		Alias:       tableAlias,
		Assignments: updates,
		From:        from,
		Where:       whereClause,
		Returning:   returning,
	}, nil
}
