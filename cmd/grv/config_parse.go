package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

type commandConstructor func(*ConfigParser, []*ConfigToken) (ConfigCommand, error)

// ConfigCommand represents a config command
type ConfigCommand interface {
	Equal(ConfigCommand) bool
}

// SetCommand contains state for setting a config variable to a value
type SetCommand struct {
	variable *ConfigToken
	value    *ConfigToken
}

// Equal returns true if the provided command is equal
func (setCommand *SetCommand) Equal(command ConfigCommand) bool {
	other, ok := command.(*SetCommand)
	if !ok {
		return false
	}

	return ((setCommand.variable != nil && setCommand.variable.Equal(other.variable)) ||
		(setCommand.variable == nil && other.variable == nil)) &&
		((setCommand.value != nil && setCommand.value.Equal(other.value)) ||
			(setCommand.value == nil && other.value == nil))
}

// ThemeCommand contains state for setting a components values for on a theme
type ThemeCommand struct {
	name      *ConfigToken
	component *ConfigToken
	bgcolor   *ConfigToken
	fgcolor   *ConfigToken
}

// Equal returns true if the provided command is equal
func (themeCommand *ThemeCommand) Equal(command ConfigCommand) bool {
	other, ok := command.(*ThemeCommand)
	if !ok {
		return false
	}

	return ((themeCommand.name != nil && themeCommand.name.Equal(other.name)) ||
		(themeCommand.name == nil && other.name == nil)) &&
		((themeCommand.component != nil && themeCommand.component.Equal(other.component)) ||
			(themeCommand.component == nil && other.component == nil)) &&
		((themeCommand.bgcolor != nil && themeCommand.bgcolor.Equal(other.bgcolor)) ||
			(themeCommand.bgcolor == nil && other.bgcolor == nil)) &&
		((themeCommand.fgcolor != nil && themeCommand.fgcolor.Equal(other.fgcolor)) ||
			(themeCommand.fgcolor == nil && other.fgcolor == nil))
}

// MapCommand contains state for mapping a key sequence to another
type MapCommand struct {
	view *ConfigToken
	from *ConfigToken
	to   *ConfigToken
}

// Equal returns true if the provided command is equal
func (mapCommand *MapCommand) Equal(command ConfigCommand) bool {
	other, ok := command.(*MapCommand)
	if !ok {
		return false
	}

	return ((mapCommand.from != nil && mapCommand.from.Equal(other.from)) ||
		(mapCommand.from == nil && other.from == nil)) &&
		((mapCommand.to != nil && mapCommand.to.Equal(other.to)) ||
			(mapCommand.to == nil && other.to == nil)) &&
		((mapCommand.view != nil && mapCommand.view.Equal(other.view)) ||
			(mapCommand.view == nil && other.view == nil))
}

// QuitCommand represents the command to quit grv
type QuitCommand struct{}

// Equal returns true if the provided command is equal
func (quitCommand *QuitCommand) Equal(command ConfigCommand) bool {
	_, ok := command.(*QuitCommand)
	return ok
}

type commandDescriptor struct {
	tokenTypes  []ConfigTokenType
	constructor commandConstructor
}

var commandDescriptors = map[string]*commandDescriptor{
	"set": {
		tokenTypes:  []ConfigTokenType{CtkWord, CtkWord},
		constructor: setCommandConstructor,
	},
	"theme": {
		tokenTypes:  []ConfigTokenType{CtkOption, CtkWord, CtkOption, CtkWord, CtkOption, CtkWord, CtkOption, CtkWord},
		constructor: themeCommandConstructor,
	},
	"map": {
		tokenTypes:  []ConfigTokenType{CtkWord, CtkWord, CtkWord},
		constructor: mapCommandConstructor,
	},
	"q": {
		tokenTypes:  []ConfigTokenType{},
		constructor: quitCommandConstructor,
	},
}

// ConfigParser is a component capable of parsing config into commands
type ConfigParser struct {
	scanner     *ConfigScanner
	inputSource string
}

// NewConfigParser creates a new ConfigParser which will read input from the provided reader
func NewConfigParser(reader io.Reader, inputSource string) *ConfigParser {
	return &ConfigParser{
		scanner:     NewConfigScanner(reader),
		inputSource: inputSource,
	}
}

// Parse returns the next command from the input stream
// eof is set to true if the end of the input stream has been reached
func (parser *ConfigParser) Parse() (command ConfigCommand, eof bool, err error) {
	var token *ConfigToken

	for {
		token, err = parser.scan()
		if err != nil {
			return
		}

		switch token.tokenType {
		case CtkWord:
			command, eof, err = parser.parseCommand(token)
		case CtkTerminator:
			continue
		case CtkEOF:
			eof = true
		case CtkOption:
			err = parser.generateParseError(token, "Unexpected Option \"%v\"", token.value)
		case CtkInvalid:
			err = parser.generateParseError(token, "Syntax Error")
		default:
			err = parser.generateParseError(token, "Unexpected token \"%v\"", token.value)
		}

		break
	}

	if err != nil {
		parser.discardTokensUntilNextCommand()
	}

	return
}

// InputSource returns the text description of the input source
func (parser *ConfigParser) InputSource() string {
	return parser.inputSource
}

func (parser *ConfigParser) scan() (token *ConfigToken, err error) {
	for {
		token, err = parser.scanner.Scan()
		if err != nil {
			return
		}

		if token.tokenType != CtkWhiteSpace && token.tokenType != CtkComment {
			break
		}
	}

	return
}

func (parser *ConfigParser) generateParseError(token *ConfigToken, errorMessage string, args ...interface{}) error {
	return generateConfigError(parser.inputSource, token, errorMessage, args...)
}

func generateConfigError(inputSource string, token *ConfigToken, errorMessage string, args ...interface{}) error {
	var buffer bytes.Buffer

	if inputSource != "" {
		buffer.WriteString(inputSource)
		buffer.WriteRune(':')
		buffer.WriteString(fmt.Sprintf("%v:%v ", token.startPos.line, token.startPos.col))
	}

	buffer.WriteString(fmt.Sprintf(errorMessage, args...))

	if token.err != nil {
		buffer.WriteString(": ")
		buffer.WriteString(token.err.Error())
	}

	return errors.New(buffer.String())
}

func (parser *ConfigParser) discardTokensUntilNextCommand() {
	for {
		token, err := parser.scan()

		if err != nil ||
			token.tokenType == CtkTerminator ||
			token.tokenType == CtkEOF {
			return
		}
	}
}

func (parser *ConfigParser) parseCommand(token *ConfigToken) (command ConfigCommand, eof bool, err error) {
	commandDescriptor, ok := commandDescriptors[token.value]
	if !ok {
		err = parser.generateParseError(token, "Invalid command \"%v\"", token.value)
		return
	}

	var tokens []*ConfigToken

	for i := 0; i < len(commandDescriptor.tokenTypes); i++ {
		token, err = parser.scan()
		expectedConfigTokenType := commandDescriptor.tokenTypes[i]

		switch {
		case err != nil:
			return
		case token.err != nil:
			err = parser.generateParseError(token, "Syntax Error")
			return
		case token.tokenType == CtkEOF:
			err = parser.generateParseError(token, "Unexpected EOF")
			eof = true
			return
		case token.tokenType != expectedConfigTokenType:
			err = parser.generateParseError(token, "Expected %v but got %v: \"%v\"",
				ConfigTokenName(expectedConfigTokenType), ConfigTokenName(token.tokenType), token.value)
			return
		}

		tokens = append(tokens, token)
	}

	command, err = commandDescriptor.constructor(parser, tokens)

	return
}

func setCommandConstructor(parser *ConfigParser, tokens []*ConfigToken) (ConfigCommand, error) {
	return &SetCommand{
		variable: tokens[0],
		value:    tokens[1],
	}, nil
}

func themeCommandConstructor(parser *ConfigParser, tokens []*ConfigToken) (ConfigCommand, error) {
	themeCommand := &ThemeCommand{}

	optionSetters := map[string]func(*ConfigToken){
		"--name":      func(name *ConfigToken) { themeCommand.name = name },
		"--component": func(component *ConfigToken) { themeCommand.component = component },
		"--bgcolor":   func(bgcolor *ConfigToken) { themeCommand.bgcolor = bgcolor },
		"--fgcolor":   func(fgcolor *ConfigToken) { themeCommand.fgcolor = fgcolor },
	}

	for i := 0; i+1 < len(tokens); i += 2 {
		optionToken := tokens[i]
		valueToken := tokens[i+1]

		optionSetter, ok := optionSetters[optionToken.value]
		if !ok {
			return nil, parser.generateParseError(optionToken, "Invalid option for theme command: \"%v\"", optionToken.value)
		}

		optionSetter(valueToken)
	}

	return themeCommand, nil
}

func mapCommandConstructor(parser *ConfigParser, tokens []*ConfigToken) (ConfigCommand, error) {
	return &MapCommand{
		view: tokens[0],
		from: tokens[1],
		to:   tokens[2],
	}, nil
}

func quitCommandConstructor(parser *ConfigParser, tokens []*ConfigToken) (ConfigCommand, error) {
	return &QuitCommand{}, nil
}
