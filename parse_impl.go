package picomatch

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	normalizeSimpleBranchRegexp = regexp.MustCompile(`^@\([^\\()[\]{}|]+\)$`)
)

func expandRange(args []string, options *Options) string {
	if options != nil && options.ExpandRange != nil {
		return options.ExpandRange(args[0], args[len(args)-1])
	}

	sort.Strings(args)
	value := "[" + strings.Join(args, "-") + "]"
	if _, err := regexp.Compile(value); err != nil {
		escaped := make([]string, len(args))
		for i, v := range args {
			escaped[i] = EscapeRegex(v)
		}
		return strings.Join(escaped, "..")
	}

	return value
}

func syntaxError(typ, char string) string {
	return fmt.Sprintf(`Missing %s: %q - use "\\%s" to match literal characters`, typ, char, char)
}

func splitTopLevel(input string) []string {
	parts := []string{}
	bracket := 0
	paren := 0
	quote := 0
	value := strings.Builder{}
	escaped := false

	for _, ch := range input {
		if escaped {
			value.WriteRune(ch)
			escaped = false
			continue
		}

		if ch == '\\' {
			value.WriteRune(ch)
			escaped = true
			continue
		}

		if ch == '"' {
			if quote == 1 {
				quote = 0
			} else {
				quote = 1
			}
			value.WriteRune(ch)
			continue
		}

		if quote == 0 {
			if ch == '[' {
				bracket++
			} else if ch == ']' && bracket > 0 {
				bracket--
			} else if bracket == 0 {
				if ch == '(' {
					paren++
				} else if ch == ')' && paren > 0 {
					paren--
				} else if ch == '|' && paren == 0 {
					parts = append(parts, value.String())
					value.Reset()
					continue
				}
			}
		}

		value.WriteRune(ch)
	}

	parts = append(parts, value.String())
	return parts
}

func isPlainBranch(branch string) bool {
	escaped := false
	for _, ch := range branch {
		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if strings.ContainsAny(string(ch), `?*+@!()[]{}\`) {
			return false
		}
	}
	return true
}

func normalizeSimpleBranch(branch string) string {
	value := strings.TrimSpace(branch)
	changed := true

	for changed {
		changed = false
		if normalizeSimpleBranchRegexp.MatchString(value) {
			value = value[2 : len(value)-1]
			changed = true
		}
	}

	if !isPlainBranch(value) {
		return ""
	}

	return regexp.MustCompile(`\\(.)`).ReplaceAllString(value, `$1`)
}

func hasRepeatedCharPrefixOverlap(branches []string) bool {
	values := make([]string, 0, len(branches))
	for _, branch := range branches {
		if normalized := normalizeSimpleBranch(branch); normalized != "" {
			values = append(values, normalized)
		}
	}

	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			a := values[i]
			b := values[j]
			if a == "" || b == "" {
				continue
			}
			char := a[0]
			if a != strings.Repeat(string(char), len(a)) || b != strings.Repeat(string(char), len(b)) {
				continue
			}

			if a == b || strings.HasPrefix(a, b) || strings.HasPrefix(b, a) {
				return true
			}
		}
	}
	return false
}

func parseRepeatedExtglob(pattern string, requireEnd bool) *ParseRepeatedExtglobMatch {
	if len(pattern) < 2 {
		return nil
	}

	if !(pattern[0] == '+' || pattern[0] == '*') || pattern[1] != '(' {
		return nil
	}

	bracket := 0
	paren := 0
	quote := 0
	escaped := false

	for i := 1; i < len(pattern); i++ {
		ch := pattern[i]
		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '"' {
			if quote == 1 {
				quote = 0
			} else {
				quote = 1
			}
			continue
		}

		if quote == 1 {
			continue
		}

		if ch == '[' {
			bracket++
			continue
		}

		if ch == ']' && bracket > 0 {
			bracket--
			continue
		}

		if bracket > 0 {
			continue
		}

		if ch == '(' {
			paren++
			continue
		}

		if ch == ')' {
			paren--
			if paren == 0 {
				if requireEnd && i != len(pattern)-1 {
					return nil
				}
				return &ParseRepeatedExtglobMatch{
					Type: string(pattern[0]),
					Body: pattern[2:i],
					End:  i,
				}
			}
		}
	}
	return nil
}

// ParseRepeatedExtglobMatch is defined in types.go

func buildCharClassStar(chars []string) string {
	if len(chars) == 1 {
		return EscapeRegex(chars[0]) + "*"
	}

	return "[" + strings.Join(chars, "") + "]*"
}

func getStarExtglobSequenceChars(pattern string) []string {
	index := 0
	chars := make([]string, 0, 4)

	for index < len(pattern) {
		match := parseRepeatedExtglob(pattern[index:], false)
		if match == nil || match.Type != "*" {
			return nil
		}

		branches := splitTopLevel(match.Body)
		if len(branches) != 1 {
			return nil
		}

		branch := strings.TrimSpace(branches[0])
		normalized := normalizeSimpleBranch(branch)
		if normalized == "" || len(normalized) != 1 {
			return nil
		}

		chars = append(chars, normalized)
		index += match.End + 1
	}

	if len(chars) < 1 {
		return nil
	}

	return chars
}

func repeatedExtglobRecursion(pattern string) int {
	depth := 0
	value := strings.TrimSpace(pattern)
	match := parseRepeatedExtglob(value, true)

	for match != nil {
		depth++
		value = strings.TrimSpace(match.Body)
		match = parseRepeatedExtglob(value, true)
	}

	return depth
}

func analyzeRepeatedExtglob(body string, options *Options) *RepeatedExtglobAnalysis {
	if options != nil && options.MaxExtglobDepth == -1 {
		return &RepeatedExtglobAnalysis{Risky: false}
	}

	max := DEFAULT_MAX_EXTGLOB_RECURSION
	if options != nil && options.MaxExtglobDepth > 0 {
		max = options.MaxExtglobDepth
	}

	branches := splitTopLevel(body)
	for i := range branches {
		branches[i] = strings.TrimSpace(branches[i])
	}

	if len(branches) > 1 {
		for _, branch := range branches {
			if branch == "" {
				return &RepeatedExtglobAnalysis{Risky: true}
			}
			if regexp.MustCompile(`^[*?]+$`).MatchString(branch) {
				return &RepeatedExtglobAnalysis{Risky: true}
			}
		}
		if hasRepeatedCharPrefixOverlap(branches) {
			return &RepeatedExtglobAnalysis{Risky: true}
		}
	}

	safeChars := make([]string, 0, len(branches))
	sawStarSequence := false
	combinable := true

	for _, branch := range branches {
		chars := getStarExtglobSequenceChars(branch)
		if chars != nil {
			sawStarSequence = true
			safeChars = append(safeChars, chars...)
			continue
		}

		literal := normalizeSimpleBranch(branch)
		if literal != "" && len(literal) == 1 {
			safeChars = append(safeChars, literal)
			continue
		}

		combinable = false
		if repeatedExtglobRecursion(branch) > max {
			return &RepeatedExtglobAnalysis{Risky: true}
		}
	}

	if sawStarSequence {
		if combinable {
			unique := make(map[string]struct{})
			safe := make([]string, 0, len(safeChars))
			for _, ch := range safeChars {
				if _, ok := unique[ch]; !ok {
					unique[ch] = struct{}{}
					safe = append(safe, ch)
				}
			}
			return &RepeatedExtglobAnalysis{Risky: true, SafeOutput: buildCharClassStar(safe)}
		}
		return &RepeatedExtglobAnalysis{Risky: true}
	}

	return &RepeatedExtglobAnalysis{Risky: false}
}

// RepeatedExtglobAnalysis is defined in types.go

func Parse(input string, options *Options) (ParseState, error) {
	opts := cloneOptions(options)
	state := ParseState{Input: input}
	cleanInput := RemovePrefix(input, &state)
	var builder strings.Builder

	segments := strings.Split(cleanInput, "/")
	for idx, segment := range segments {
		if idx > 0 {
			builder.WriteByte('/')
		}
		if segment == "" {
			continue
		}

		if idx == 0 && !opts.Dot && len(segment) > 0 && (segment[0] == '*' || segment[0] == '?') {
			builder.WriteString(`[^/\.]`)
		}

		for i := 0; i < len(segment); i++ {
			ch := segment[i]
			switch ch {
			case '*':
				builder.WriteString(`[^/]*`)
			case '?':
				builder.WriteString(`[^/]`)
			case '.':
				builder.WriteString(`\.`)
			default:
				builder.WriteString(EscapeRegex(string(ch)))
			}
		}
	}

	state.Output = builder.String()
	return state, nil
}

func parseLegacy(input string, options *Options) (ParseState, error) {
	if options != nil && options.Noext {
		options.Noextglob = true
	}

	if input == "" {
		return ParseState{}, nil
	}

	opts := cloneOptions(options)
	max := MAX_LENGTH
	if opts.MaxLength > 0 {
		if opts.MaxLength < max {
			max = opts.MaxLength
		}
	}

	if len(input) > max {
		return ParseState{}, fmt.Errorf("Input length: %d, exceeds maximum allowed length: %d", len(input), max)
	}

	bos := &ParseToken{Type: "bos", Value: "", Output: opts.Prepend}
	tokens := []*ParseToken{bos}
	capture := "?:"
	if opts.Capture {
		capture = ""
	}

	chars := GetGlobChars(opts.Windows)
	_ = ExtglobChars(chars)

	DOT_LITERAL := chars.DotLiteral
	PLUS_LITERAL := chars.PlusLiteral
	SLASH_LITERAL := chars.SlashLiteral
	ONE_CHAR := chars.OneChar
	DOTS_SLASH := chars.DotsSlash
	NO_DOT := chars.NoDot
	NO_DOT_SLASH := chars.NoDotSlash
	NO_DOTS_SLASH := chars.NoDotsSlash
	QMARK := chars.Qmark
	QMARK_NO_DOT := chars.QmarkNoDot
	STAR := chars.Star
	START_ANCHOR := chars.StartAnchor

	globstar := func(o *Options) string {
		pattern := START_ANCHOR
		if o.Dot {
			pattern += DOTS_SLASH
		} else {
			pattern += DOT_LITERAL
		}
		return "(" + capture + `(?:(?!` + pattern + `).)*?)`
	}

	nodot := ""
	if !opts.Dot {
		nodot = NO_DOT
	}
	qmarkNoDot := QMARK_NO_DOT
	if opts.Dot {
		qmarkNoDot = QMARK
	}
	star := nodot + STAR
	if opts.Bash {
		star = globstar(opts)
	}
	if opts.Capture {
		star = "(" + star + ")"
	}

	if opts.Noext {
		opts.Noextglob = true
	}

	state := ParseState{
		Input:     input,
		Index:     -1,
		Start:     0,
		Dot:       opts.Dot,
		Consumed:  "",
		Output:    "",
		Prefix:    "",
		Backtrack: false,
		Negated:   false,
		Brackets:  0,
		Braces:    0,
		Parens:    0,
		Quotes:    0,
		Globstar:  false,
		Tokens:    tokens,
	}

	input = RemovePrefix(input, &state)
	length := len(input)

	extglobs := []*ParseToken{}
	braces := []*ParseToken{}
	stack := []string{}
	prev := bos

	eeos := func() bool { return state.Index == length-1 }
	eos := func() bool { return state.Index >= length }
	eof := func() bool { return state.Index >= len(input) }
	peek := func(n int) byte {
		if state.Index+n < len(input) {
			return input[state.Index+n]
		}
		return 0
	}
	advance := func() byte {
		state.Index++
		if state.Index >= len(input) {
			return 0
		}
		return input[state.Index]
	}
	remaining := func() string { return input[state.Index+1:] }
	consume := func(value string, num int) {
		state.Consumed += value
		state.Index += num
	}
	appendOutput := func(token *ParseToken) {
		state.Output += token.Output
		consume(token.Value, len(token.Value))
	}

	negate := func() bool {
		count := 1
		for peek(1) == '!' && (peek(2) != '(' || peek(3) == '?') {
			advance()
			state.Start++
			count++
		}
		if count%2 == 0 {
			return false
		}
		state.Negated = true
		state.Start++
		return true
	}

	increment := func(typ string) {
		switch typ {
		case "brackets":
			state.Brackets++
		case "braces":
			state.Braces++
		case "parens":
			state.Parens++
		case "quotes":
			state.Quotes++
		}
		stack = append(stack, typ)
	}
	decrement := func(typ string) {
		switch typ {
		case "brackets":
			state.Brackets--
		case "braces":
			state.Braces--
		case "parens":
			state.Parens--
		case "quotes":
			state.Quotes--
		}
		if len(stack) > 0 {
			stack = stack[:len(stack)-1]
		}
	}

	push := func(tok *ParseToken) {
		if prev.Type == "globstar" {
			isBrace := state.Braces > 0 && (tok.Type == "comma" || tok.Type == "brace")
			isExtglob := tok.Extglob || (len(extglobs) > 0 && (tok.Type == "pipe" || tok.Type == "paren"))
			if tok.Type != "slash" && tok.Type != "paren" && !isBrace && !isExtglob {
				state.Output = state.Output[:len(state.Output)-len(prev.Output)]
				prev.Type = "star"
				prev.Value = "*"
				prev.Output = star
				state.Output += prev.Output
			}
		}

		if len(extglobs) > 0 && tok.Type != "paren" {
			extglobs[len(extglobs)-1].Inner += tok.Value
		}

		if tok.Value != "" || tok.Output != "" {
			appendOutput(tok)
		}

		if prev != nil && prev.Type == "text" && tok.Type == "text" {
			prev.Output = prev.Output + tok.Value
			prev.Value += tok.Value
			return
		}

		tok.Prev = prev
		tok.TokensIndex = len(tokens)
		tokens = append(tokens, tok)
		prev = tok
	}

	extglobOpen := func(t, value string) {
		token := &ParseToken{
			Type:       t,
			Value:      value,
			Output:     "",
			Conditions: 1,
			Inner:      "",
		}

		token.Prev = prev
		token.Parens = state.Parens
		token.Output = state.Output
		token.StartIndex = state.Index
		token.TokensIndex = len(tokens)
		output := ""
		if opts.Capture {
			output = "(" + token.Open
		} else {
			output = token.Open
		}

		increment("parens")
		push(&ParseToken{Type: t, Value: value, Output: state.Output})
		push(&ParseToken{Type: "paren", Extglob: true, Value: string(advance()), Output: output})
		extglobs = append(extglobs, token)
	}

	extglobClose := func(token *ParseToken) {
		literal := input[token.StartIndex : state.Index+1]
		body := input[token.StartIndex+2 : state.Index]
		analysis := analyzeRepeatedExtglob(body, opts)

		if (token.Type == "plus" || token.Type == "star") && analysis.Risky {
			safeOutput := analysis.SafeOutput
			if safeOutput != "" {
				if token.Output != "" {
					safeOutput = ""
				}
				if opts.Capture {
					safeOutput = "(" + safeOutput + ")"
				}
			}
			open := tokens[token.TokensIndex]
			open.Type = "text"
			open.Value = literal
			open.Output = safeOutput
			if open.Output == "" {
				open.Output = EscapeRegex(literal)
			}

			for i := token.TokensIndex + 1; i < len(tokens); i++ {
				tokens[i].Value = ""
				tokens[i].Output = ""
				tokens[i].Suffix = ""
			}

			state.Output = token.Output + open.Output
			state.Backtrack = true

			push(&ParseToken{Type: "paren", Extglob: true, Value: string(token.Value[0]), Output: ""})
			decrement("parens")
			return
		}

		output := token.Close
		if opts.Capture {
			output = output + ")"
		}
		var rest string
		if token.Type == "negate" {
			extglobStar := star

			if token.Inner != "" && strings.Contains(token.Inner, "/") {
				extglobStar = globstar(opts)
			}

			if extglobStar != star || eeos() || regexp.MustCompile(`^\)+$`).MatchString(remaining()) {
				output = token.Close + `)` + `)$` + extglobStar
			}

			if strings.Contains(token.Inner, "*") {
				rest = remaining()
				if regexp.MustCompile(`^\.[^\\/.]+$`).MatchString(rest) {
					expression, err := Parse(rest, options)
					if err == nil {
						output = token.Close + expression.Output + `)` + extglobStar + `)`
					}
				}
			}

			if token.Prev != nil && token.Prev.Type == "bos" {
				state.NegatedExtglob = true
			}
		}

		push(&ParseToken{Type: "paren", Extglob: true, Value: string(token.Value[0]), Output: output})
		decrement("parens")
	}

	fastpathDisabled := strings.HasPrefix(input, "*") || strings.HasPrefix(input, "!") || strings.ContainsAny(input, "/()[]{}\"")
	if opts.Fastpaths && !fastpathDisabled {
		output := input
		backslashes := false
		var builder strings.Builder
		lastIndex := 0
		for _, loc := range REGEX_SPECIAL_CHARS_BACKREF.FindAllStringSubmatchIndex(input, -1) {
			builder.WriteString(input[lastIndex:loc[0]])
			esc := input[loc[2]:loc[3]]
			chars := input[loc[4]:loc[5]]
			first := input[loc[6]:loc[7]]
			rest := input[loc[8]:loc[9]]
			if first == `\\` {
				backslashes = true
				builder.WriteString(input[loc[0]:loc[1]])
			} else if first == "?" {
				if esc != "" {
					builder.WriteString(esc + first + strings.Repeat(QMARK, len(rest)))
				} else if loc[0] == 0 {
					builder.WriteString(qmarkNoDot + strings.Repeat(QMARK, len(rest)))
				} else {
					builder.WriteString(strings.Repeat(QMARK, len(chars)))
				}
			} else if first == "." {
				builder.WriteString(strings.Repeat(DOT_LITERAL, len(chars)))
			} else if first == "*" {
				if esc != "" {
					builder.WriteString(esc + first)
					if len(rest) > 0 {
						builder.WriteString(star)
					}
				} else {
					builder.WriteString(star)
				}
			} else {
				if esc != "" {
					builder.WriteString(input[loc[0]:loc[1]])
				} else {
					builder.WriteString(`\` + input[loc[0]:loc[1]])
				}
			}
			lastIndex = loc[1]
		}
		builder.WriteString(input[lastIndex:])
		output = builder.String()

		if backslashes {
			if opts.Unescape {
				output = strings.ReplaceAll(output, "\\", "")
			} else {
				output = regexp.MustCompile(`\\+`).ReplaceAllStringFunc(output, func(m string) string {
					if len(m)%2 == 0 {
						return `\\`
					}
					return `\\`
				})
			}
		}

		if output == input && opts.Contains {
			state.Output = input
			return state, nil
		}

		state.Output = WrapOutput(output, &state, options)
		return state, nil
	}

	for !eos() {
		ch := advance()
		if ch == 0 {
			break
		}
		if ch == '\u0000' {
			continue
		}

		value := string(ch)

		if ch == '\\' {
			next := peek(1)
			if next == CHAR_FORWARD_SLASH && !opts.Bash {
				continue
			}
			if next == CHAR_DOT || next == CHAR_SEMICOLON {
				continue
			}
			if next == 0 {
				push(&ParseToken{Type: "text", Value: string(ch) + "\\"})
			}

			match := regexp.MustCompile(`^\\+`).FindString(remaining())
			slashesCount := 0
			if len(match) > 2 {
				slashesCount = len(match)
				state.Index += slashesCount
				if slashesCount%2 != 0 {
					// prepare string value with trailing backslash
					// will be adjusted below
				}
			}

			var sval string
			if opts.Unescape {
				sval = string(advance())
			} else {
				sval = string(ch) + string(advance())
			}

			if state.Brackets == 0 {
				push(&ParseToken{Type: "text", Value: sval})
				continue
			}
		}

		if state.Brackets > 0 && (ch != ']' || prev.Value == "[" || prev.Value == "[^") {
			if opts.Posix && ch == ':' {
				inner := prev.Value[1:]
				if strings.Contains(inner, "[") {
					prev.Posix = true
					if strings.Contains(inner, ":") {
						idx := strings.LastIndex(prev.Value, "[")
						pre := prev.Value[:idx]
						rest := prev.Value[idx+2:]
						if posix, ok := PosixRegexSource[rest]; ok {
							prev.Value = pre + posix
							state.Backtrack = true
							advance()
							if state.Output == "" && len(tokens) > 1 && tokens[1] == prev {
								tokens[0].Output = ONE_CHAR
							}
							continue
						}
					}
				}
			}

			var value string
			if ch == '[' && peek(1) != ':' || (ch == '-' && peek(1) == ']') {
				value = `\` + string(ch)
			} else if ch == ']' && (prev.Value == "[" || prev.Value == "[^") {
				value = `\` + string(ch)
			} else if opts.Posix && ch == '!' && prev.Value == "[" {
				value = "^"
			}

			if value != "" {
				prev.Value += value
				appendOutput(&ParseToken{Value: value})
			} else {
				prev.Value += string(ch)
				appendOutput(&ParseToken{Value: string(ch)})
			}
			continue
		}

		if state.Quotes == 1 && ch != '"' {
			valueStr := EscapeRegex(string(ch))
			prev.Value += valueStr
			appendOutput(&ParseToken{Value: valueStr})
			continue
		}

		if value == "\"" {
			if state.Quotes == 1 {
				state.Quotes = 0
			} else {
				state.Quotes = 1
			}
			if opts.KeepQuotes {
				push(&ParseToken{Type: "text", Value: string(value)})
			}
			continue
		}

		if value == "(" {
			increment("parens")
			push(&ParseToken{Type: "paren", Value: string(value)})
			continue
		}

		if value == ")" {
			if state.Parens == 0 && opts.StrictBrackets {
				return ParseState{}, fmt.Errorf(syntaxError("opening", "("))
			}

			if len(extglobs) > 0 && state.Parens == extglobs[len(extglobs)-1].Parens+1 {
				extglobClose(extglobs[len(extglobs)-1])
				extglobs = extglobs[:len(extglobs)-1]
				continue
			}

			push(&ParseToken{Type: "paren", Value: string(value), Output: ")"})
			decrement("parens")
			continue
		}

		if value == "[" {
			if opts.Nobracket || !strings.Contains(remaining(), "]") {
				if !opts.Nobracket && opts.StrictBrackets {
					return ParseState{}, fmt.Errorf(syntaxError("closing", "]"))
				}
				value = "["
			} else {
				increment("brackets")
			}

			push(&ParseToken{Type: "bracket", Value: string(value), Output: "["})
			continue
		}

		if value == "]" {
			if opts.Nobracket || (prev != nil && prev.Type == "bracket" && len(prev.Value) == 1) {
				push(&ParseToken{Type: "text", Value: string(value), Output: `\]`})
				continue
			}

			if state.Brackets == 0 {
				if opts.StrictBrackets {
					return ParseState{}, fmt.Errorf(syntaxError("opening", "["))
				}
				push(&ParseToken{Type: "text", Value: string(value), Output: `\]`})
				continue
			}

			decrement("brackets")
			prevValue := prev.Value[1:]
			if !prev.Posix && strings.HasPrefix(prevValue, "^") && !strings.Contains(prevValue, "/") {
				value = "/"
			}

			prev.Value += string(value)
			appendOutput(&ParseToken{Value: string(value)})

			if opts.LiteralBrackets == false || HasRegexChars(prevValue) {
				continue
			}

			escaped := EscapeRegex(prev.Value)
			state.Output = state.Output[:len(state.Output)-len(prev.Value)]
			if opts.LiteralBrackets == true {
				state.Output += escaped
				prev.Value = escaped
				continue
			}

			prev.Value = "(" + capture + escaped + "|" + prev.Value + ")"
			state.Output += prev.Value
			continue
		}

		if value == "{" && !opts.Nobrace {
			increment("braces")
			open := &ParseToken{Type: "brace", Value: string(value), Output: "(", OutputIndex: len(state.Output), TokensIndex: len(tokens)}
			braces = append(braces, open)
			push(open)
			continue
		}

		if value == "}" {
			brace := (*ParseToken)(nil)
			if len(braces) > 0 {
				brace = braces[len(braces)-1]
			}

			if opts.Nobrace || brace == nil {
				push(&ParseToken{Type: "text", Value: string(value), Output: string(value)})
				continue
			}

			output := ")"
			if brace.Dots {
				arr := append([]*ParseToken(nil), tokens...)
				rangeTokens := []string{}
				for i := len(arr) - 1; i >= 0; i-- {
					if tokens[i].Type == "brace" {
						break
					}
					if tokens[i].Type != "dots" {
						rangeTokens = append([]string{tokens[i].Value}, rangeTokens...)
					}
				}
				output = expandRange(rangeTokens, opts)
				state.Backtrack = true
			}

			if !brace.Comma && !brace.Dots {
				out := state.Output[:brace.OutputIndex]
				toks := state.Tokens[brace.TokensIndex:]
				brace.Value = "{"
				brace.Output = "("
				value = "}"
				output = "}"
				state.Output = out
				for _, t := range toks {
					if t.Output != "" {
						state.Output += t.Output
					} else {
						state.Output += t.Value
					}
				}
			}

			push(&ParseToken{Type: "brace", Value: string(value), Output: output})
			decrement("braces")
			braces = braces[:len(braces)-1]
			continue
		}

		if value == "|" {
			if len(extglobs) > 0 {
				extglobs[len(extglobs)-1].Conditions++
			}
			push(&ParseToken{Type: "text", Value: string(value)})
			continue
		}

		if value == "," {
			output := string(value)
			brace := (*ParseToken)(nil)
			if len(braces) > 0 {
				brace = braces[len(braces)-1]
			}
			if brace != nil && stack[len(stack)-1] == "braces" {
				brace.Comma = true
				output = "|"
			}
			push(&ParseToken{Type: "comma", Value: string(value), Output: output})
			continue
		}

		if value == "/" {
			if prev.Type == "dot" && state.Index == state.Start+1 {
				state.Start = state.Index + 1
				state.Consumed = ""
				state.Output = ""
				tokens = tokens[:len(tokens)-1]
				prev = bos
				continue
			}
			push(&ParseToken{Type: "slash", Value: string(value), Output: SLASH_LITERAL})
			continue
		}

		if value == "." {
			if state.Braces > 0 && prev.Type == "dot" {
				if prev.Value == ".." {
					prev.Output = DOT_LITERAL
				}
				brace := braces[len(braces)-1]
				prev.Type = "dots"
				prev.Output += string(value)
				prev.Value += string(value)
				brace.Dots = true
				continue
			}

			if state.Braces+state.Parens == 0 && prev.Type != "bos" && prev.Type != "slash" {
				push(&ParseToken{Type: "text", Value: string(value), Output: DOT_LITERAL})
				continue
			}

			push(&ParseToken{Type: "dot", Value: string(value), Output: DOT_LITERAL})
			continue
		}

		if value == "?" {
			isGroup := prev != nil && prev.Value == "("
			if !isGroup && !opts.Noextglob && peek(1) == '(' && peek(2) != '?' {
				extglobOpen("qmark", "?")
				continue
			}

			if prev != nil && prev.Type == "paren" {
				next := peek(1)
				output := string(value)
				if (prev.Value == "(" && !regexp.MustCompile(`[!=<:]`).MatchString(string(next))) || (next == '<' && !regexp.MustCompile(`<([!=]|\w+>)`).MatchString(remaining())) {
					output = `\?`
				}
				push(&ParseToken{Type: "text", Value: string(value), Output: output})
				continue
			}

			if !opts.Dot && (prev.Type == "slash" || prev.Type == "bos") {
				push(&ParseToken{Type: "qmark", Value: string(value), Output: QMARK_NO_DOT})
				continue
			}

			push(&ParseToken{Type: "qmark", Value: string(value), Output: QMARK})
			continue
		}

		if value == "!" {
			if !opts.Noextglob && peek(1) == '(' {
				if peek(2) != '?' || !regexp.MustCompile(`[!=<:]`).MatchString(string(peek(3))) {
					extglobOpen("negate", "!")
					continue
				}
			}

			if !opts.Nonegate && state.Index == 0 {
				negate()
				continue
			}
		}

		if value == "+" {
			if !opts.Noextglob && peek(1) == '(' && peek(2) != '?' {
				extglobOpen("plus", "+")
				continue
			}

			if (prev != nil && prev.Value == "(") || !opts.Regex {
				push(&ParseToken{Type: "plus", Value: string(value), Output: PLUS_LITERAL})
				continue
			}

			if prev != nil && (prev.Type == "bracket" || prev.Type == "paren" || prev.Type == "brace") || state.Parens > 0 {
				push(&ParseToken{Type: "plus", Value: string(value)})
				continue
			}

			push(&ParseToken{Type: "plus", Value: PLUS_LITERAL, Output: PLUS_LITERAL})
			continue
		}

		if value == "@" {
			if !opts.Noextglob && peek(1) == '(' && peek(2) != '?' {
				push(&ParseToken{Type: "at", Extglob: true, Value: string(value), Output: ""})
				continue
			}
			push(&ParseToken{Type: "text", Value: string(value)})
			continue
		}

		if value != "*" {
			if value == "$" || value == "^" {
				value = "\\" + value
			}

			if match := REGEX_NON_SPECIAL_CHARS.FindString(remaining()); match != "" {
				value = string(value) + match
				state.Index += len(match)
			}

			push(&ParseToken{Type: "text", Value: string(value)})
			continue
		}

		if prev != nil && (prev.Type == "globstar" || prev.Star) {
			prev.Type = "star"
			prev.Star = true
			prev.Value += string(value)
			prev.Output = star
			state.Backtrack = true
			state.Globstar = true
			consume(string(value), 1)
			continue
		}

		rest := remaining()
		if !opts.Noextglob && strings.HasPrefix(rest, "(") && len(rest) > 1 && rest[1] != '?' {
			extglobOpen("star", "*")
			continue
		}

		if prev != nil && prev.Type == "star" {
			if opts.NoGlobstar {
				consume(string(value), 1)
				continue
			}

			prior := prev.Prev
			before := (*ParseToken)(nil)
			if prior != nil {
				before = prior.Prev
			}
			isStart := prior != nil && (prior.Type == "slash" || prior.Type == "bos")
			afterStar := before != nil && (before.Type == "star" || before.Type == "globstar")

			if opts.Bash && (!isStart || (len(rest) > 0 && rest[0] != '/')) {
				push(&ParseToken{Type: "star", Value: string(value), Output: ""})
				continue
			}

			isBrace := state.Braces > 0 && (prior.Type == "comma" || prior.Type == "brace")
			isExtglob := len(extglobs) > 0 && (prior.Type == "pipe" || prior.Type == "paren")
			if !isStart && prior.Type != "paren" && !isBrace && !isExtglob {
				push(&ParseToken{Type: "star", Value: string(value), Output: ""})
				continue
			}

			for strings.HasPrefix(rest, "/**") {
				after := byte(0)
				if state.Index+4 < len(input) {
					after = input[state.Index+4]
				}
				if after != '/' {
					break
				}
				rest = rest[3:]
				consume("/**", 3)
			}

			if prior.Type == "bos" && eof() {
				prev.Type = "globstar"
				prev.Value += string(value)
				prev.Output = globstar(opts)
				state.Output = prev.Output
				state.Globstar = true
				consume(string(value), 1)
				continue
			}

			if prior.Type == "slash" && prior.Prev != nil && prior.Prev.Type != "bos" && !afterStar && eof() {
				state.Output = state.Output[:len(state.Output)-len(prior.Output)-len(prev.Output)]
				prior.Output = "(?:" + prior.Output

				prev.Type = "globstar"
				prev.Output = globstar(opts) + ")"
				if !opts.StrictSlashes {
					prev.Output += "|$)"
				}
				prev.Value += string(value)
				state.Globstar = true
				state.Output += prior.Output + prev.Output
				consume(string(value), 1)
				continue
			}

			if prior.Type == "slash" && prior.Prev != nil && prior.Prev.Type != "bos" && strings.HasPrefix(rest, "/") {
				end := ""
				if len(rest) > 1 {
					end = "|$"
				}

				state.Output = state.Output[:len(state.Output)-len(prior.Output)-len(prev.Output)]
				prior.Output = "(?:" + prior.Output

				prev.Type = "globstar"
				prev.Output = globstar(opts) + SLASH_LITERAL + "|" + SLASH_LITERAL + end + ")"
				prev.Value += string(value)
				state.Output += prior.Output + prev.Output
				consume(string(value), 1)
				advance()
				push(&ParseToken{Type: "slash", Value: "/", Output: ""})
				continue
			}

			if prior.Type == "bos" && strings.HasPrefix(rest, "/") {
				prev.Type = "globstar"
				prev.Value += string(value)
				prev.Output = "(?:^|" + SLASH_LITERAL + "|" + globstar(opts) + SLASH_LITERAL + ")"
				state.Output = prev.Output
				state.Globstar = true
				consume(string(value), 1)
				advance()
				push(&ParseToken{Type: "slash", Value: "/", Output: ""})
				continue
			}

			state.Output = state.Output[:len(state.Output)-len(prev.Output)]
			prev.Type = "globstar"
			prev.Output = globstar(opts)
			prev.Value += string(value)
			state.Output += prev.Output
			state.Globstar = true
			consume(string(value), 1)
			continue
		}

		token := &ParseToken{Type: "star", Value: string(value), Output: star}
		if opts.Bash {
			token.Output = ".*?"
			if prev.Type == "bos" || prev.Type == "slash" {
				token.Output = nodot + token.Output
			}
			push(token)
			continue
		}

		if prev != nil && (prev.Type == "bracket" || prev.Type == "paren") && opts.Regex {
			token.Output = string(value)
			push(token)
			continue
		}

		if state.Index == state.Start || prev.Type == "slash" || prev.Type == "dot" {
			if prev.Type == "dot" {
				state.Output += NO_DOT_SLASH
				prev.Output += NO_DOT_SLASH
			} else if opts.Dot {
				state.Output += NO_DOTS_SLASH
				prev.Output += NO_DOTS_SLASH
			} else {
				state.Output += nodot
				prev.Output += nodot
			}

			if peek(1) != CHAR_ASTERISK {
				state.Output += ONE_CHAR
				prev.Output += ONE_CHAR
			}
		}

		push(token)
	}

	for state.Brackets > 0 {
		if opts.StrictBrackets {
			return ParseState{}, fmt.Errorf(syntaxError("closing", "]"))
		}
		state.Output = EscapeLast(state.Output, "[", len(state.Output)-1)
		decrement("brackets")
	}

	for state.Parens > 0 {
		if opts.StrictBrackets {
			return ParseState{}, fmt.Errorf(syntaxError("closing", ")"))
		}
		state.Output = EscapeLast(state.Output, "(", len(state.Output)-1)
		decrement("parens")
	}

	for state.Braces > 0 {
		if opts.StrictBrackets {
			return ParseState{}, fmt.Errorf(syntaxError("closing", "}"))
		}
		state.Output = EscapeLast(state.Output, "{", len(state.Output)-1)
		decrement("braces")
	}

	if !opts.StrictSlashes && (prev.Type == "star" || prev.Type == "bracket") {
		push(&ParseToken{Type: "maybe_slash", Value: "", Output: SLASH_LITERAL + "?"})
	}

	if state.Backtrack {
		state.Output = ""
		for _, token := range state.Tokens {
			if token.Output != "" {
				state.Output += token.Output
			} else {
				state.Output += token.Value
			}
			if token.Suffix != "" {
				state.Output += token.Suffix
			}
		}
	}

	return state, nil
}

func parseFastpaths(input string, options *Options) (string, error) {
	return "", nil
}

func parseFastpathsLegacy(input string, options *Options) (string, error) {
	opts := cloneOptions(options)
	max := MAX_LENGTH
	if opts.MaxLength > 0 && opts.MaxLength < max {
		max = opts.MaxLength
	}
	if len(input) > max {
		return "", fmt.Errorf("Input length: %d, exceeds maximum allowed length: %d", len(input), max)
	}

	input = Replacements[input]

	chars := GetGlobChars(opts.Windows)
	DOT_LITERAL := chars.DotLiteral
	SLASH_LITERAL := chars.SlashLiteral
	ONE_CHAR := chars.OneChar
	DOTS_SLASH := chars.DotsSlash
	NO_DOT := chars.NoDot
	NO_DOTS := chars.NoDots
	NO_DOTS_SLASH := chars.NoDotsSlash
	STAR := chars.Star
	START_ANCHOR := chars.StartAnchor

	nodot := NO_DOTS
	if opts.Dot {
		nodot = NO_DOT
	}
	slashDot := NO_DOTS_SLASH
	if opts.Dot {
		slashDot = NO_DOT_SLASH
	}
	capture := "?:"
	if opts.Capture {
		capture = ""
	}

	state := ParseState{Negated: false, Prefix: ""}
	star := STAR
	if opts.Bash {
		star = ".*?"
	}
	if opts.Capture {
		star = "(" + star + ")"
	}

	globstar := func(o *Options) string {
		if o.NoGlobstar {
			return star
		}
		pattern := START_ANCHOR
		if o.Dot {
			pattern += DOTS_SLASH
		} else {
			pattern += DOT_LITERAL
		}
		return "(" + capture + `(?:(?!` + pattern + `).)*?)`
	}

	var create func(str string) string
	create = func(str string) string {
		switch str {
		case "*":
			return nodot + ONE_CHAR + star
		case ".*":
			return DOT_LITERAL + ONE_CHAR + star
		case "*.*":
			return nodot + star + DOT_LITERAL + ONE_CHAR + star
		case "*/*":
			return nodot + star + SLASH_LITERAL + ONE_CHAR + slashDot + star
		case "**":
			return nodot + globstar(opts)
		case "**/*":
			return "(?:" + nodot + globstar(opts) + SLASH_LITERAL + ")?" + slashDot + ONE_CHAR + star
		case "**/*.*":
			return "(?:" + nodot + globstar(opts) + SLASH_LITERAL + ")?" + slashDot + star + DOT_LITERAL + ONE_CHAR + star
		case "**/.*":
			return "(?:" + nodot + globstar(opts) + SLASH_LITERAL + ")?" + DOT_LITERAL + ONE_CHAR + star
		default:
			if idx := strings.LastIndex(str, "."); idx != -1 {
				source := create(str[:idx])
				if source == "" {
					return ""
				}
				return source + DOT_LITERAL + str[idx+1:]
			}
			return ""
		}
	}

	output := RemovePrefix(input, &state)
	source := create(output)
	if source != "" {
		if !opts.StrictSlashes {
			source += SLASH_LITERAL + "?"
		}
	}

	return source, nil
}

func cloneOptions(options *Options) *Options {
	if options == nil {
		return &Options{}
	}
	clone := *options
	return &clone
}
