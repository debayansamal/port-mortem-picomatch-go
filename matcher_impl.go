package picomatch

import (
	"errors"
	"regexp"
	"strings"
)

func Test(input string, regex *regexp.Regexp, options *Options, glob string, posix bool) (bool, []string, string, error) {
	if input == "" {
		return false, nil, "", nil
	}

	opts := options
	if opts == nil {
		opts = &Options{}
	}

	var format func(string) string
	if opts.Format != nil {
		format = opts.Format
	} else if posix {
		format = ToPosixSlashes
	}

	match := input == glob
	output := input
	if match && format != nil {
		output = format(input)
	}

	if !match {
		if format != nil {
			output = format(input)
		}
		match = output == glob
	}

	var found []string
	if !match || opts.Capture {
		if opts.MatchBase || opts.Basename {
			match = MatchBase(input, regex, options, posix)
		} else {
			found = regex.FindStringSubmatch(output)
			match = found != nil
		}
	}

	return match, found, output, nil
}

func MatchBase(input string, globOrRegex interface{}, options *Options, posix bool) bool {
	switch v := globOrRegex.(type) {
	case *regexp.Regexp:
		return v.MatchString(Basename(input, posix))
	case string:
		r, err := regexp.Compile(v)
		if err == nil {
			return r.MatchString(Basename(input, posix))
		}
		regex, err := MakeRe(v, options)
		if err != nil {
			return false
		}
		return regex.MatchString(Basename(input, posix))
	default:
		return false
	}
}

func IsMatch(str string, patterns interface{}, options *Options) (bool, error) {
	opts := options
	if opts == nil {
		opts = &Options{}
	}

	switch p := patterns.(type) {
	case string:
		pattern := p
		negated := false
		if strings.HasPrefix(pattern, "!") && !opts.Nonegate {
			negated = true
			pattern = pattern[1:]
		}

		input := str
		if (opts.MatchBase || opts.Basename) && !strings.Contains(pattern, "/") {
			input = Basename(str, opts.Windows)
		}

		regex, err := MakeRe(pattern, opts)
		if err != nil {
			return false, err
		}

		matched := regex.MatchString(input)
		if negated {
			return !matched, nil
		}
		return matched, nil

	case []string:
		for _, pattern := range p {
			ok, err := IsMatch(str, pattern, opts)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, errors.New("patterns must be string or []string")
	}
}

func CompileRe(state ParseState, options *Options) (*regexp.Regexp, error) {
	opts := options
	if opts == nil {
		opts = &Options{}
	}

	prepend := "^"
	append := "$"
	if opts.Contains {
		prepend = ""
		append = ""
	}

	source := prepend + "(?:" + state.Output + ")" + append
	if state.Negated {
		source = `^(?!` + source + `).*$`
	}

	return ToRegex(source, options)
}

func ToRegex(source string, options *Options) (*regexp.Regexp, error) {
	opts := options
	if opts == nil {
		opts = &Options{}
	}

	if opts.Flags != "" {
		if strings.Contains(opts.Flags, "i") {
			source = `(?i:` + source + `)`
		}
	} else if opts.Nocase {
		source = `(?i:` + source + `)`
	}

	return regexp.Compile(source)
}

func MakeRe(input string, options *Options) (*regexp.Regexp, error) {
	if input == "" {
		return nil, errors.New("expected a non-empty string")
	}

	opts := options
	if opts == nil {
		opts = &Options{}
	}

	var output string
	if opts.Fastpaths {
		s, err := parseFastpaths(input, options)
		if err == nil && s != "" {
			output = s
		}
	}

	var state ParseState
	var err error
	if output == "" {
		state, err = Parse(input, options)
		if err != nil {
			return nil, err
		}
	} else {
		state = ParseState{Output: output, Negated: false}
	}

	return CompileRe(state, options)
}
