// Package gomysqlx is a MySQL-specialized SQL parser forked from GoSQLX.
// It provides tokenization and parsing of MySQL SQL into an AST.
package gomysqlx

import (
	"fmt"

	"github.com/unoflavora/gomysqlx/ast"
	"github.com/unoflavora/gomysqlx/parser"
	"github.com/unoflavora/gomysqlx/tokenizer"
)

// Parse tokenizes and parses a MySQL SQL statement, returning an AST.
func Parse(sql string) (*ast.AST, error) {
	tkz := tokenizer.GetTokenizer()
	defer tokenizer.PutTokenizer(tkz)

	tokens, err := tkz.Tokenize([]byte(sql))
	if err != nil {
		return nil, fmt.Errorf("tokenization failed: %w", err)
	}

	p := parser.GetParser()
	defer parser.PutParser(p)

	astNode, err := p.ParseFromModelTokens(tokens)
	if err != nil {
		return nil, fmt.Errorf("parsing failed: %w", err)
	}

	return astNode, nil
}
