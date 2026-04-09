// Copyright 2026 GoSQLX Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"strings"

	"github.com/unoflavora/gomysqlx/ast"
	"github.com/unoflavora/gomysqlx/models"
)

// parseAlterStatement parses ALTER statements
func (p *Parser) parseAlterStatement() (*ast.AlterStatement, error) {
	stmt := ast.GetAlterStatement()

	// Parse the type of object being altered
	switch {
	case p.matchType(models.TokenTypeTable):
		stmt.Type = ast.AlterTypeTable
		return p.parseAlterTableStatement(stmt)
	case p.matchType(models.TokenTypeRole):
		stmt.Type = ast.AlterTypeRole
		return p.parseAlterRoleStatement(stmt)
	case p.matchType(models.TokenTypePolicy):
		stmt.Type = ast.AlterTypePolicy
		return p.parseAlterPolicyStatement(stmt)
	case p.matchType(models.TokenTypeConnector):
		stmt.Type = ast.AlterTypeConnector
		return p.parseAlterConnectorStatement(stmt)
	default:
		return nil, p.expectedError("TABLE, ROLE, POLICY, or CONNECTOR")
	}
}

// parseAlterTableStatement parses ALTER TABLE statements with full MySQL support.
// Supports multiple comma-separated operations, MODIFY, CHANGE, ADD INDEX,
// DROP INDEX/PRIMARY KEY/FOREIGN KEY, ENGINE, CHARACTER SET, AUTO_INCREMENT,
// ALGORITHM, LOCK, FORCE, DISABLE/ENABLE KEYS, ORDER BY, CONVERT TO, ALTER INDEX.
func (p *Parser) parseAlterTableStatement(stmt *ast.AlterStatement) (*ast.AlterStatement, error) {
	stmt.Name = p.parseIdentAsString()

	// Parse first operation
	op, err := p.parseAlterTableOperation()
	if err != nil {
		return nil, err
	}
	stmt.Operation = op

	// Parse additional comma-separated operations (MySQL supports multiple)
	for p.isType(models.TokenTypeComma) {
		p.advance() // Consume comma
		_, err := p.parseAlterTableOperation()
		if err != nil {
			return nil, err
		}
		// We only store the first operation in stmt.Operation for backward compatibility.
		// Additional operations are consumed but not stored in the AST (sufficient for
		// table name extraction which is our primary use case).
	}

	// Parse optional ALGORITHM and LOCK
	for p.isTokenMatch("ALGORITHM") || p.isTokenMatch("LOCK") {
		p.advance()
		if p.isType(models.TokenTypeEq) {
			p.advance()
		}
		if p.isIdentifier() || p.isTokenMatch("NONE") || p.isTokenMatch("SHARED") ||
			p.isTokenMatch("EXCLUSIVE") || p.isTokenMatch("DEFAULT") ||
			p.isTokenMatch("INPLACE") || p.isTokenMatch("COPY") || p.isTokenMatch("INSTANT") {
			p.advance()
		}
	}

	return stmt, nil
}

func (p *Parser) parseAlterTableOperation() (*ast.AlterTableOperation, error) {
	op := &ast.AlterTableOperation{}

	switch {
	case p.matchType(models.TokenTypeAdd):
		// ADD [COLUMN] col_def | ADD CONSTRAINT | ADD [UNIQUE|FULLTEXT|SPATIAL] INDEX/KEY
		if p.matchType(models.TokenTypeColumn) {
			op.Type = ast.AddColumn
			colDef, err := p.parseColumnDef()
			if err != nil {
				return nil, err
			}
			op.ColumnDef = colDef
		} else if p.matchType(models.TokenTypeConstraint) {
			op.Type = ast.AddConstraint
			constraint, err := p.parseTableConstraint()
			if err != nil {
				return nil, err
			}
			op.Constraint = constraint
		} else if p.isAnyType(models.TokenTypeIndex, models.TokenTypeKey) ||
			p.isType(models.TokenTypeUnique) || p.isTokenMatch("FULLTEXT") || p.isTokenMatch("SPATIAL") {
			// ADD [UNIQUE|FULLTEXT|SPATIAL] INDEX/KEY name (columns)
			op.Type = ast.AddConstraint
			// Skip index type modifiers
			for p.isType(models.TokenTypeUnique) || p.isTokenMatch("FULLTEXT") || p.isTokenMatch("SPATIAL") {
				p.advance()
			}
			if p.isAnyType(models.TokenTypeIndex, models.TokenTypeKey) {
				p.advance()
			}
			// Skip index name if present
			if p.isIdentifier() {
				p.advance()
			}
			// Parse column list
			if p.isType(models.TokenTypeLParen) {
				p.advance()
				for !p.isType(models.TokenTypeRParen) && !p.isType(models.TokenTypeEOF) {
					p.advance()
				}
				if p.isType(models.TokenTypeRParen) {
					p.advance()
				}
			}
		} else if p.isAnyType(models.TokenTypePrimary, models.TokenTypeForeign) {
			op.Type = ast.AddConstraint
			constraint, err := p.parseTableConstraint()
			if err != nil {
				return nil, err
			}
			op.Constraint = constraint
		} else {
			// MySQL allows ADD col_def without COLUMN keyword
			op.Type = ast.AddColumn
			colDef, err := p.parseColumnDef()
			if err != nil {
				return nil, err
			}
			op.ColumnDef = colDef
		}

	case p.matchType(models.TokenTypeDrop):
		if p.matchType(models.TokenTypeColumn) {
			op.Type = ast.DropColumn
			ident := p.parseIdent()
			if ident == nil {
				return nil, p.expectedError("column name")
			}
			op.ColumnName = &ast.Ident{Name: ident.Name}
			if p.matchType(models.TokenTypeCascade) {
				op.CascadeDrops = true
			}
		} else if p.matchType(models.TokenTypeConstraint) {
			op.Type = ast.DropConstraint
			ident := p.parseIdent()
			if ident == nil {
				return nil, p.expectedError("constraint name")
			}
			op.ConstraintName = &ast.Ident{Name: ident.Name}
			if p.matchType(models.TokenTypeCascade) {
				op.CascadeDrops = true
			}
		} else if p.isAnyType(models.TokenTypeIndex, models.TokenTypeKey) {
			p.advance() // INDEX/KEY
			op.Type = ast.DropConstraint
			ident := p.parseIdent()
			if ident != nil {
				op.ConstraintName = &ast.Ident{Name: ident.Name}
			}
		} else if p.isType(models.TokenTypePrimary) {
			p.advance() // PRIMARY
			if p.isType(models.TokenTypeKey) {
				p.advance() // KEY
			}
			op.Type = ast.DropConstraint
			op.ConstraintName = &ast.Ident{Name: "PRIMARY"}
		} else if p.isType(models.TokenTypeForeign) {
			p.advance() // FOREIGN
			if p.isType(models.TokenTypeKey) {
				p.advance() // KEY
			}
			op.Type = ast.DropConstraint
			ident := p.parseIdent()
			if ident != nil {
				op.ConstraintName = &ast.Ident{Name: ident.Name}
			}
		} else {
			// DROP column_name (without COLUMN keyword)
			op.Type = ast.DropColumn
			ident := p.parseIdent()
			if ident == nil {
				return nil, p.expectedError("COLUMN, INDEX, PRIMARY KEY, FOREIGN KEY, or column name")
			}
			op.ColumnName = &ast.Ident{Name: ident.Name}
		}

	case p.isTokenMatch("MODIFY"):
		p.advance() // MODIFY
		if p.matchType(models.TokenTypeColumn) {
			// optional COLUMN keyword
		}
		op.Type = ast.AlterColumn
		colDef, err := p.parseColumnDef()
		if err != nil {
			return nil, err
		}
		op.ColumnDef = colDef

	case p.isTokenMatch("CHANGE"):
		p.advance() // CHANGE
		if p.matchType(models.TokenTypeColumn) {
			// optional COLUMN keyword
		}
		op.Type = ast.RenameColumn
		// Old column name
		ident := p.parseIdent()
		if ident == nil {
			return nil, p.expectedError("old column name")
		}
		op.ColumnName = &ast.Ident{Name: ident.Name}
		// New column definition (includes new name + type + constraints)
		colDef, err := p.parseColumnDef()
		if err != nil {
			return nil, err
		}
		op.ColumnDef = colDef
		op.NewColumnName = &ast.Ident{Name: colDef.Name}

	case p.matchType(models.TokenTypeRename):
		if p.matchType(models.TokenTypeTo) || p.isType(models.TokenTypeAs) {
			if p.isType(models.TokenTypeAs) {
				p.advance()
			}
			op.Type = ast.RenameTable
			op.NewTableName = p.parseObjectName()
		} else if p.matchType(models.TokenTypeColumn) {
			op.Type = ast.RenameColumn
			ident := p.parseIdent()
			if ident == nil {
				return nil, p.expectedError("column name")
			}
			op.ColumnName = &ast.Ident{Name: ident.Name}
			if !p.matchType(models.TokenTypeTo) {
				return nil, p.expectedError("TO")
			}
			newIdent := p.parseIdent()
			if newIdent == nil {
				return nil, p.expectedError("new column name")
			}
			op.NewColumnName = &ast.Ident{Name: newIdent.Name}
		} else if p.isAnyType(models.TokenTypeIndex, models.TokenTypeKey) {
			p.advance() // INDEX/KEY
			op.Type = ast.RenameColumn // reuse for index rename
			ident := p.parseIdent()
			if ident != nil {
				op.ColumnName = &ast.Ident{Name: ident.Name}
			}
			if p.matchType(models.TokenTypeTo) {
				newIdent := p.parseIdent()
				if newIdent != nil {
					op.NewColumnName = &ast.Ident{Name: newIdent.Name}
				}
			}
		} else {
			op.Type = ast.RenameTable
			op.NewTableName = p.parseObjectName()
		}

	case p.matchType(models.TokenTypeAlter):
		if p.matchType(models.TokenTypeColumn) {
			op.Type = ast.AlterColumn
			ident := p.parseIdent()
			if ident == nil {
				return nil, p.expectedError("column name")
			}
			op.ColumnName = &ast.Ident{Name: ident.Name}
			colDef, err := p.parseColumnDef()
			if err != nil {
				return nil, err
			}
			op.ColumnDef = colDef
		} else if p.isAnyType(models.TokenTypeIndex, models.TokenTypeKey) {
			// ALTER INDEX idx_name INVISIBLE/VISIBLE
			p.advance()
			op.Type = ast.AlterColumn
			ident := p.parseIdent()
			if ident != nil {
				op.ColumnName = &ast.Ident{Name: ident.Name}
			}
			// Consume INVISIBLE/VISIBLE
			if p.isTokenMatch("INVISIBLE") || p.isTokenMatch("VISIBLE") {
				p.advance()
			}
		} else {
			return nil, p.expectedError("COLUMN or INDEX")
		}

	case p.isTokenMatch("CONVERT"):
		// CONVERT TO CHARACTER SET charset [COLLATE collation]
		p.advance() // CONVERT
		if p.matchType(models.TokenTypeTo) {
			// skip CHARACTER SET / CHARSET + value
			for !p.isType(models.TokenTypeEOF) && !p.isType(models.TokenTypeSemicolon) &&
				!p.isType(models.TokenTypeComma) {
				p.advance()
			}
		}
		op.Type = ast.AlterColumn

	default:
		// Handle MySQL table-level options: ENGINE, AUTO_INCREMENT, FORCE, DISABLE/ENABLE KEYS, ORDER BY
		val := strings.ToUpper(p.currentToken.Token.Value)
		switch val {
		case "ENGINE", "AUTO_INCREMENT", "ROW_FORMAT", "CHARSET", "CHARACTER":
			op.Type = ast.AlterColumn // use as placeholder
			p.advance()
			if p.isType(models.TokenTypeEq) {
				p.advance()
			}
			if p.isIdentifier() || p.isNumericLiteral() || p.isType(models.TokenTypeString) {
				p.advance()
			}
		case "FORCE":
			op.Type = ast.AlterColumn
			p.advance()
		case "DISABLE", "ENABLE":
			op.Type = ast.AlterColumn
			p.advance()
			if p.isTokenMatch("KEYS") {
				p.advance()
			}
		case "ORDER":
			op.Type = ast.AlterColumn
			p.advance()
			if p.isType(models.TokenTypeBy) {
				p.advance()
				// Parse order expression
				for {
					if p.isIdentifier() {
						p.advance()
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
		default:
			return nil, p.expectedError("ADD, DROP, MODIFY, CHANGE, RENAME, ALTER, CONVERT, ENGINE, FORCE, or other ALTER TABLE operation")
		}
	}

	return op, nil
}

// parseAlterRoleStatement parses ALTER ROLE statements
func (p *Parser) parseAlterRoleStatement(stmt *ast.AlterStatement) (*ast.AlterStatement, error) {
	stmt.Name = p.parseIdentAsString()
	op := &ast.AlterRoleOperation{}

	switch {
	case p.matchType(models.TokenTypeRename):
		if !p.matchType(models.TokenTypeTo) {
			return nil, p.expectedError("TO")
		}
		op.Type = ast.RenameRole
		op.NewName = p.parseIdentAsString()

	case p.matchType(models.TokenTypeAdd):
		if !p.matchType(models.TokenTypeMember) {
			return nil, p.expectedError("MEMBER")
		}
		op.Type = ast.AddMember
		op.MemberName = p.parseIdentAsString()

	case p.matchType(models.TokenTypeDrop):
		if !p.matchType(models.TokenTypeMember) {
			return nil, p.expectedError("MEMBER")
		}
		op.Type = ast.DropMember
		op.MemberName = p.parseIdentAsString()

	case p.matchType(models.TokenTypeSet):
		op.Type = ast.SetConfig
		op.ConfigName = p.parseIdentAsString()
		if p.matchType(models.TokenTypeTo) || p.matchType(models.TokenTypeEq) {
			expr, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			op.ConfigValue = expr
		}

	case p.matchType(models.TokenTypeReset):
		op.Type = ast.ResetConfig
		if p.matchType(models.TokenTypeAll) {
			op.ConfigName = "ALL"
		} else {
			op.ConfigName = p.parseIdentAsString()
		}

	case p.matchType(models.TokenTypeWith):
		op.Type = ast.WithOptions
		for {
			option, err := p.parseRoleOption()
			if err != nil {
				return nil, err
			}
			op.Options = append(op.Options, *option)
			if !p.matchType(models.TokenTypeComma) {
				break
			}
		}

	default:
		return nil, p.expectedError("RENAME, ADD MEMBER, DROP MEMBER, SET, RESET, or WITH")
	}

	stmt.Operation = op
	return stmt, nil
}

// parseAlterPolicyStatement parses ALTER POLICY statements
func (p *Parser) parseAlterPolicyStatement(stmt *ast.AlterStatement) (*ast.AlterStatement, error) {
	stmt.Name = p.parseIdentAsString()
	if !p.matchType(models.TokenTypeOn) {
		return nil, p.expectedError("ON")
	}
	p.parseIdentAsString() // table name

	op := &ast.AlterPolicyOperation{}

	if p.matchType(models.TokenTypeRename) {
		if !p.matchType(models.TokenTypeTo) {
			return nil, p.expectedError("TO")
		}
		op.Type = ast.RenamePolicy
		op.NewName = p.parseIdentAsString()
	} else {
		op.Type = ast.ModifyPolicy
		if p.matchType(models.TokenTypeTo) {
			for {
				op.To = append(op.To, p.parseIdentAsString())
				if !p.matchType(models.TokenTypeComma) {
					break
				}
			}
		}
		if p.matchType(models.TokenTypeUsing) {
			if !p.matchType(models.TokenTypeLParen) {
				return nil, p.expectedError("(")
			}
			expr, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			op.Using = expr
			if !p.matchType(models.TokenTypeRParen) {
				return nil, p.expectedError(")")
			}
		}
		if p.matchType(models.TokenTypeWith) && p.matchType(models.TokenTypeCheck) {
			if !p.matchType(models.TokenTypeLParen) {
				return nil, p.expectedError("(")
			}
			expr, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			op.WithCheck = expr
			if !p.matchType(models.TokenTypeRParen) {
				return nil, p.expectedError(")")
			}
		}
	}

	stmt.Operation = op
	return stmt, nil
}

// parseAlterConnectorStatement parses ALTER CONNECTOR statements
func (p *Parser) parseAlterConnectorStatement(stmt *ast.AlterStatement) (*ast.AlterStatement, error) {
	stmt.Name = p.parseIdentAsString()
	if !p.matchType(models.TokenTypeSet) {
		return nil, p.expectedError("SET")
	}

	op := &ast.AlterConnectorOperation{}

	switch {
	case p.matchType(models.TokenTypeDcproperties):
		if !p.matchType(models.TokenTypeLParen) {
			return nil, p.expectedError("(")
		}
		op.Properties = make(map[string]string)
		for {
			// DCPROPERTIES keys are arbitrary user-defined names that may
			// coincide with SQL keywords (e.g. "key", "url").  Accept any
			// word-like token as a bare identifier here.
			key := p.parseBareWordAsString()
			if !p.matchType(models.TokenTypeEq) {
				return nil, p.expectedError("=")
			}
			value := p.parseStringLiteral()
			op.Properties[key] = value

			if !p.matchType(models.TokenTypeComma) {
				break
			}
		}
		if !p.matchType(models.TokenTypeRParen) {
			return nil, p.expectedError(")")
		}

	case p.matchType(models.TokenTypeUrl):
		op.URL = p.parseStringLiteral()

	case p.matchType(models.TokenTypeOwner):
		owner := &ast.AlterConnectorOwner{}
		if p.matchType(models.TokenTypeUser) {
			owner.IsUser = true
		} else if p.matchType(models.TokenTypeRole) {
			owner.IsUser = false
		} else {
			return nil, p.expectedError("USER or ROLE")
		}
		owner.Name = p.parseIdentAsString()
		op.Owner = owner

	default:
		return nil, p.expectedError("DCPROPERTIES, URL, or OWNER")
	}

	stmt.Operation = op
	return stmt, nil
}

// parseRoleOption parses a role option
func (p *Parser) parseRoleOption() (*ast.RoleOption, error) {
	option := &ast.RoleOption{}

	switch {
	case p.isType(models.TokenTypeSuperuser):
		option.Name = "SUPERUSER"
		option.Type = ast.SuperUser
		option.Value = true
		p.advance()
	case p.isType(models.TokenTypeNosuperuser):
		option.Name = "SUPERUSER"
		option.Type = ast.SuperUser
		option.Value = false
		p.advance()

	case p.isType(models.TokenTypeCreateDB):
		option.Name = "CREATEDB"
		option.Type = ast.CreateDB
		option.Value = true
		p.advance()
	case p.isType(models.TokenTypeNocreatedb):
		option.Name = "CREATEDB"
		option.Type = ast.CreateDB
		option.Value = false
		p.advance()

	case p.isType(models.TokenTypeCreateRole):
		option.Name = "CREATEROLE"
		option.Type = ast.CreateRole
		option.Value = true
		p.advance()
	case p.isType(models.TokenTypeNocreaterole):
		option.Name = "CREATEROLE"
		option.Type = ast.CreateRole
		option.Value = false
		p.advance()

	case p.isType(models.TokenTypeLogin):
		option.Name = "LOGIN"
		option.Type = ast.Login
		option.Value = true
		p.advance()
	case p.isType(models.TokenTypeNologin):
		option.Name = "LOGIN"
		option.Type = ast.Login
		option.Value = false
		p.advance()

	case p.matchType(models.TokenTypePassword):
		option.Name = "PASSWORD"
		option.Type = ast.Password
		if p.matchType(models.TokenTypeNull) {
			option.Value = nil
		} else {
			expr, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			option.Value = expr
		}

	case p.matchType(models.TokenTypeValid):
		option.Name = "VALID"
		option.Type = ast.ValidUntil
		if !p.matchType(models.TokenTypeUntil) {
			return nil, p.expectedError("UNTIL")
		}
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		option.Value = expr

	default:
		return nil, p.expectedError("role option")
	}

	return option, nil
}
