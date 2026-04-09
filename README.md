# gomysqlx

A battle-hardened MySQL SQL parser for Go. Forked from [GoSQLX](https://github.com/ajitpratap0/GoSQLX) and specialized for MySQL 5.7 through 8.4.

GoSQLX is a great multi-dialect parser, but it chokes on many common MySQL patterns: `UPDATE ... JOIN`, `INSERT IGNORE`, `CREATE TABLE` with `ENGINE`/`CHARSET`/`COLLATE`, `DELETE ... JOIN`, and more. **gomysqlx fixes all of them.**

## What's Fixed

| Category | Patterns Fixed |
|----------|---------------|
| **UPDATE** | JOIN, INNER/LEFT/RIGHT JOIN, comma join, LOW_PRIORITY, IGNORE, ORDER BY LIMIT, correlated subquery in SET, CTE + UPDATE |
| **DELETE** | JOIN, multi-table DELETE, USING syntax, LOW_PRIORITY, QUICK, IGNORE, ORDER BY LIMIT, PARTITION |
| **INSERT** | IGNORE, LOW_PRIORITY, HIGH_PRIORITY, DELAYED, SET syntax, DEFAULT in VALUES, PARTITION |
| **REPLACE** | REPLACE ... SELECT, LOW_PRIORITY, DELAYED |
| **CREATE TABLE** | ENGINE, DEFAULT CHARSET, COLLATE, COMMENT, AUTO_INCREMENT, UNSIGNED, ON UPDATE CURRENT_TIMESTAMP, GENERATED ALWAYS AS, ENUM/SET types, IF NOT EXISTS with complex columns, PARTITION BY RANGE/HASH/KEY, CREATE TABLE ... LIKE, CREATE TABLE ... AS SELECT |
| **CREATE INDEX** | FULLTEXT, SPATIAL, INVISIBLE/VISIBLE, functional indexes `((UPPER(col)))`, descending indexes |
| **ALTER TABLE** | MODIFY COLUMN, CHANGE COLUMN, multiple comma-separated operations, ADD INDEX, DROP INDEX/PRIMARY KEY/FOREIGN KEY, AFTER, CONVERT TO CHARACTER SET, ENGINE, AUTO_INCREMENT, FORCE, DISABLE/ENABLE KEYS, ALTER INDEX INVISIBLE, ALGORITHM/LOCK options |
| **Statements** | DESC (alias for DESCRIBE), EXPLAIN table (as DESCRIBE), VALUES ROW() (MySQL 8.0.19+), TABLE statement (MySQL 8.0.19+), parenthesized queries `(SELECT ...) UNION ALL (SELECT ...)` |

**46 previously-failing MySQL patterns now parse correctly**, with zero regressions on GoSQLX's existing test suite.

## Install

```bash
go get github.com/unoflavora/gomysqlx
```

## Usage

```go
package main

import (
    "fmt"
    "github.com/unoflavora/gomysqlx"
    "github.com/unoflavora/gomysqlx/ast"
)

func main() {
    // Parses everything MySQL throws at it
    result, err := gomysqlx.Parse(`
        UPDATE users u
        JOIN orders o ON u.id = o.user_id
        SET u.total_spent = (SELECT SUM(amount) FROM payments WHERE user_id = u.id)
        WHERE o.status = 'completed'
    `)
    if err != nil {
        panic(err)
    }

    stmt := result.Statements[0]
    switch s := stmt.(type) {
    case *ast.UpdateStatement:
        fmt.Println("Table:", s.TableName)  // "users"
        fmt.Println("Joins:", len(s.From))  // joined tables
    }
}
```

## API

```go
// Parse tokenizes and parses a MySQL SQL statement into an AST.
func Parse(sql string) (*ast.AST, error)
```

Returns the same AST types as GoSQLX (`ast.SelectStatement`, `ast.InsertStatement`, `ast.UpdateStatement`, `ast.DeleteStatement`, `ast.CreateTableStatement`, etc.) so it's a **drop-in replacement** -- just change the import path.

## Why Fork Instead of Rewrite

GoSQLX already handles ~87% of MySQL syntax correctly: SELECT with window functions, CTEs, subqueries, UNION/INTERSECT/EXCEPT, JSON functions, CASE expressions, and more. The failures were in specific DML/DDL parser files that didn't implement MySQL-specific syntax extensions. Forking let us fix the broken 13% while keeping the proven 87%.

## Upstream

Forked from [GoSQLX v1.13.0](https://github.com/ajitpratap0/GoSQLX) (Apache 2.0 license). All original GoSQLX tests pass.

## License

Apache License 2.0 (same as GoSQLX).
