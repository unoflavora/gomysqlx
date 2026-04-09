// Copyright 2026 GoSQLX Authors
// Copyright 2026 gomysqlx Authors (MySQL extensions)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Package parser - dml_delete.go
// DELETE statement parsing with full MySQL support.
//
// MySQL syntax:
//
// Single-table:
//
//	DELETE [LOW_PRIORITY] [QUICK] [IGNORE] FROM tbl_name [PARTITION (...)]
//	[WHERE ...] [ORDER BY ...] [LIMIT row_count]
//
// Multi-table (syntax 1):
//
//	DELETE [LOW_PRIORITY] [QUICK] [IGNORE] tbl_name[.*] [, tbl_name[.*] ...]
//	FROM table_references [WHERE ...]
//
// Multi-table (syntax 2):
//
//	DELETE [LOW_PRIORITY] [QUICK] [IGNORE] FROM tbl_name[.*] [, tbl_name[.*] ...]
//	USING table_references [WHERE ...]

package parser

import (
	"strings"

	"github.com/unoflavora/gomysqlx/ast"
	"github.com/unoflavora/gomysqlx/models"
)

// parseDeleteStatement parses a MySQL DELETE statement.
func (p *Parser) parseDeleteStatement() (ast.Statement, error) {
	// We've already consumed the DELETE token in matchType

	// Skip MySQL modifiers: LOW_PRIORITY, QUICK, IGNORE
	for p.isTokenMatch("LOW_PRIORITY") || p.isTokenMatch("QUICK") || p.isTokenMatch("IGNORE") {
		p.advance()
	}

	// Determine which DELETE syntax we're looking at:
	// 1. DELETE FROM table ...              (single-table)
	// 2. DELETE FROM t1, t2 USING ...       (multi-table syntax 2)
	// 3. DELETE t1 FROM t1 JOIN t2 ...      (multi-table syntax 1)

	var tableName string
	var alias string
	var using []ast.TableReference
	var joins []ast.JoinClause

	if p.isType(models.TokenTypeFrom) {
		p.advance() // Consume FROM

		// Parse first table name
		name, err := p.parseQualifiedName()
		if err != nil {
			return nil, p.expectedError("table name")
		}
		tableName = name

		// Check if this is multi-table context: alias + comma/USING/JOIN follows.
		// For single-table DELETE, don't consume an alias (could eat RETURNING, WHERE, etc.)
		if p.isIdentifier() && !p.isType(models.TokenTypeWhere) {
			// Peek ahead: if what follows the identifier is a comma, JOIN, USING, or FROM,
			// then this identifier is a table alias
			nextVal := strings.ToUpper(p.currentToken.Token.Value)
			if nextVal != "WHERE" && nextVal != "RETURNING" && nextVal != "PARTITION" &&
				nextVal != "ORDER" && nextVal != "LIMIT" {
				alias = p.currentToken.Token.Value
				p.advance()
			}
		}

		firstRef := ast.TableReference{Name: tableName, Alias: alias}

		// Check for comma-separated targets (multi-table syntax 2): DELETE FROM t1, t2 USING ...
		// or PARTITION clause, or single-table with WHERE/ORDER BY/LIMIT
		if p.isType(models.TokenTypeComma) {
			// Multi-table syntax 2: DELETE FROM t1, t2 USING table_references
			targets := []ast.TableReference{firstRef}
			for p.isType(models.TokenTypeComma) {
				p.advance()
				ref, err := p.parseFromTableReference()
				if err != nil {
					return nil, err
				}
				targets = append(targets, ref)
			}
			// Expect USING
			if p.isTokenMatch("USING") {
				p.advance()
				// Parse table references after USING (with JOINs)
				ref, err := p.parseFromTableReference()
				if err != nil {
					return nil, err
				}
				using = append(using, ref)
				// More comma-separated tables
				for p.isType(models.TokenTypeComma) {
					p.advance()
					ref, err := p.parseFromTableReference()
					if err != nil {
						return nil, err
					}
					using = append(using, ref)
				}
				// JOIN clauses after USING tables
				if p.isJoinKeyword() {
					joins, err = p.parseJoinClauses(using[0])
					if err != nil {
						return nil, err
					}
					for _, j := range joins {
						using = append(using, j.Right)
					}
				}
			}
		} else if p.isTokenMatch("USING") {
			// Single target DELETE FROM t1 USING t1 JOIN t2
			p.advance()
			ref, err := p.parseFromTableReference()
			if err != nil {
				return nil, err
			}
			using = append(using, ref)
			for p.isType(models.TokenTypeComma) {
				p.advance()
				ref, err := p.parseFromTableReference()
				if err != nil {
					return nil, err
				}
				using = append(using, ref)
			}
			if p.isJoinKeyword() {
				joins, err = p.parseJoinClauses(using[0])
				if err != nil {
					return nil, err
				}
				for _, j := range joins {
					using = append(using, j.Right)
				}
			}
		} else {
			// Single-table DELETE FROM t1 [PARTITION (...)] [WHERE] [ORDER BY] [LIMIT]
			// Skip PARTITION clause if present
			if p.isTokenMatch("PARTITION") {
				p.advance()
				if p.isType(models.TokenTypeLParen) {
					p.advance()
					for !p.isType(models.TokenTypeRParen) && !p.isType(models.TokenTypeEOF) {
						p.advance()
					}
					if p.isType(models.TokenTypeRParen) {
						p.advance()
					}
				}
			}
			// Check for JOIN after single table (e.g. DELETE FROM t1 JOIN t2 is not standard
			// but handle gracefully by treating as using)
		}
	} else {
		// Multi-table syntax 1: DELETE t1 [, t2] FROM table_references
		// Or: DELETE t1.* FROM ...
		firstRef, err := p.parseFromTableReference()
		if err != nil {
			return nil, p.expectedError("table name or FROM")
		}
		tableName = firstRef.Name
		alias = firstRef.Alias

		// Handle .* (DELETE t1.*)
		if p.isType(models.TokenTypeDot) {
			p.advance()
			if p.isTokenMatch("*") || p.isType(models.TokenTypeMul) {
				p.advance()
			}
		}

		// More target tables
		for p.isType(models.TokenTypeComma) {
			p.advance()
			ref, err := p.parseFromTableReference()
			if err != nil {
				return nil, err
			}
			using = append(using, ref)
			// Handle .*
			if p.isType(models.TokenTypeDot) {
				p.advance()
				if p.isTokenMatch("*") || p.isType(models.TokenTypeMul) {
					p.advance()
				}
			}
		}

		// Expect FROM
		if !p.isType(models.TokenTypeFrom) {
			return nil, p.expectedError("FROM")
		}
		p.advance()

		// Parse table references (with JOINs)
		ref, err := p.parseFromTableReference()
		if err != nil {
			return nil, err
		}
		using = append(using, ref)
		// For multi-table DELETE, set tableName to the first real table from FROM clause
		tableName = ref.Name
		for p.isType(models.TokenTypeComma) {
			p.advance()
			ref, err := p.parseFromTableReference()
			if err != nil {
				return nil, err
			}
			using = append(using, ref)
		}
		if p.isJoinKeyword() {
			joins, err = p.parseJoinClauses(ref)
			if err != nil {
				return nil, err
			}
			for _, j := range joins {
				using = append(using, j.Right)
			}
		}
	}

	// Parse WHERE clause if present
	var whereClause ast.Expression
	if p.isType(models.TokenTypeWhere) {
		p.advance() // Consume WHERE
		var err error
		whereClause, err = p.parseExpression()
		if err != nil {
			return nil, err
		}
	}

	// Parse ORDER BY clause if present (MySQL single-table only)
	if p.isType(models.TokenTypeOrder) {
		p.advance() // Consume ORDER
		if p.isType(models.TokenTypeBy) {
			p.advance() // Consume BY
			for {
				_, err := p.parseExpression()
				if err != nil {
					return nil, err
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

	// Parse LIMIT clause if present (MySQL)
	if p.isType(models.TokenTypeLimit) {
		p.advance() // Consume LIMIT
		if p.isNumericLiteral() {
			p.advance()
		}
	}

	// Parse RETURNING clause if present (PostgreSQL)
	var returning []ast.Expression
	if p.isType(models.TokenTypeReturning) || p.isTokenMatch("RETURNING") {
		p.advance()
		var err error
		returning, err = p.parseReturningColumns()
		if err != nil {
			return nil, err
		}
	}

	return &ast.DeleteStatement{
		TableName: tableName,
		Alias:     alias,
		Using:     using,
		Where:     whereClause,
		Returning: returning,
	}, nil
}
