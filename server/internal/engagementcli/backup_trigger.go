package engagementcli

import (
	"fmt"
	"strings"
	"unicode"
)

// scopedTriggerDependencies accepts the protective trigger subset used by the
// financial modules. It is intentionally conservative, not a general SQL parser:
// comments, DML, routines, qualified tables and ambiguous FROM lists fail closed.
// Literal text is skipped according to the trigger's original SQL mode.
func scopedTriggerDependencies(body, mode string, tables []string) error {
	tokens, e := triggerTokens(body, mode)
	if e != nil {
		return e
	}
	selected := map[string]bool{}
	for _, table := range tables {
		selected[table] = true
	}
	functions := map[string]bool{"IF": true, "NOT": true, "AND": true, "OR": true, "IN": true, "EXISTS": true, "COUNT": true, "COALESCE": true, "CAST": true, "JSON_UNQUOTE": true, "JSON_EXTRACT": true}
	depth := 0
	fromDepth := map[int]bool{}
	for i, token := range tokens {
		word := strings.ToUpper(token)
		switch word {
		case "CALL", "PREPARE", "EXECUTE", "INSERT", "UPDATE", "DELETE", "REPLACE", "CREATE", "ALTER", "DROP", "LOAD", "HANDLER", "WITH", "UNION", "INTO", "TABLE":
			return fmt.Errorf("selected trigger has unsupported dependencies")
		case "FROM", "JOIN":
			if i+1 >= len(tokens) || !selected[strings.Trim(tokens[i+1], "`")] || (i+2 < len(tokens) && tokens[i+2] == ".") {
				return fmt.Errorf("selected trigger references a table outside backup scope")
			}
			fromDepth[depth] = true
		case "WHERE", "GROUP", "ORDER", "HAVING", "LIMIT", ";":
			delete(fromDepth, depth)
		case ",":
			if fromDepth[depth] {
				return fmt.Errorf("selected trigger has ambiguous table dependencies")
			}
		case "(":
			depth++
		case ")":
			delete(fromDepth, depth)
			depth--
			if depth < 0 {
				return fmt.Errorf("selected trigger syntax unsupported")
			}
		}
		if i+1 < len(tokens) && tokens[i+1] == "(" && identifier.MatchString(strings.Trim(token, "`")) {
			if strings.HasPrefix(token, "`") || !functions[word] || (i > 0 && tokens[i-1] == ".") {
				return fmt.Errorf("selected trigger calls an unsupported function")
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("selected trigger syntax unsupported")
	}
	return nil
}

func triggerTokens(body, mode string) ([]string, error) {
	ansi := strings.Contains(strings.ToUpper(mode), "ANSI_QUOTES")
	noBackslash := strings.Contains(strings.ToUpper(mode), "NO_BACKSLASH_ESCAPES")
	var tokens []string
	for i := 0; i < len(body); {
		if body[i] >= 0x80 || body[i] == '$' || body[i] == '@' {
			return nil, fmt.Errorf("selected trigger token unsupported")
		}
		if unicode.IsSpace(rune(body[i])) {
			i++
			continue
		}
		if body[i] == '#' || (i+1 < len(body) && (body[i:i+2] == "/*" || body[i:i+2] == "--")) {
			return nil, fmt.Errorf("selected trigger comments unsupported")
		}
		if body[i] == '\'' || body[i] == '`' || body[i] == '"' {
			quote := body[i]
			quotedIdentifier := quote == '`' || (quote == '"' && ansi)
			i++
			var value strings.Builder
			closed := false
			for i < len(body) {
				if body[i] == quote {
					if i+1 < len(body) && body[i+1] == quote {
						value.WriteByte(quote)
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				if !quotedIdentifier && !noBackslash && body[i] == '\\' {
					i++
					if i >= len(body) {
						break
					}
				}
				value.WriteByte(body[i])
				i++
			}
			if !closed {
				return nil, fmt.Errorf("selected trigger quoted value incomplete")
			}
			if quotedIdentifier {
				if !identifier.MatchString(value.String()) {
					return nil, fmt.Errorf("selected trigger identifier unsupported")
				}
				tokens = append(tokens, "`"+value.String()+"`")
			} else {
				tokens = append(tokens, "'literal'")
			}
			continue
		}
		start := i
		for i < len(body) && ((body[i] >= 'a' && body[i] <= 'z') || (body[i] >= 'A' && body[i] <= 'Z') || (body[i] >= '0' && body[i] <= '9') || body[i] == '_') {
			i++
		}
		if start == i {
			i++
		}
		tokens = append(tokens, body[start:i])
	}
	return tokens, nil
}
