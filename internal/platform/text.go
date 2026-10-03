package platform

import (
	"fmt"
	"time"
)

func TypeText(d InputDriver, text string, interval time.Duration) error {
	for _, r := range text {
		key, mods, ok := textRune(r)
		if !ok {
			return fmt.Errorf("text character %q is not supported by the JACoB keyboard mapper", r)
		}
		if err := d.TapChord(key, mods); err != nil {
			return err
		}
		if interval > 0 {
			time.Sleep(interval)
		}
	}
	return nil
}

func textRune(r rune) (string, []string, bool) {
	if r >= 'a' && r <= 'z' {
		return string(r - 'a' + 'A'), nil, true
	}
	if r >= 'A' && r <= 'Z' {
		return string(r), []string{"SHIFT"}, true
	}
	if r >= '0' && r <= '9' {
		return string(r), nil, true
	}
	switch r {
	case ' ':
		return "SPACE", nil, true
	case '-':
		return "MINUS", nil, true
	case '_':
		return "MINUS", []string{"SHIFT"}, true
	case '=':
		return "EQUALS", nil, true
	case '+':
		return "EQUALS", []string{"SHIFT"}, true
	case '.':
		return "PERIOD", nil, true
	case ',':
		return "COMMA", nil, true
	case '/':
		return "SLASH", nil, true
	case '?':
		return "SLASH", []string{"SHIFT"}, true
	case '\\':
		return "BACKSLASH", nil, true
	case ';':
		return "SEMICOLON", nil, true
	case ':':
		return "SEMICOLON", []string{"SHIFT"}, true
	case '\'':
		return "APOSTROPHE", nil, true
	case '"':
		return "APOSTROPHE", []string{"SHIFT"}, true
	case '[':
		return "LEFTBRACKET", nil, true
	case '{':
		return "LEFTBRACKET", []string{"SHIFT"}, true
	case ']':
		return "RIGHTBRACKET", nil, true
	case '}':
		return "RIGHTBRACKET", []string{"SHIFT"}, true
	case '!':
		return "1", []string{"SHIFT"}, true
	case '@':
		return "2", []string{"SHIFT"}, true
	case '#':
		return "3", []string{"SHIFT"}, true
	case '$':
		return "4", []string{"SHIFT"}, true
	case '%':
		return "5", []string{"SHIFT"}, true
	case '^':
		return "6", []string{"SHIFT"}, true
	case '&':
		return "7", []string{"SHIFT"}, true
	case '*':
		return "8", []string{"SHIFT"}, true
	case '(':
		return "9", []string{"SHIFT"}, true
	case ')':
		return "0", []string{"SHIFT"}, true
	case '\n':
		return "ENTER", nil, true
	case '\t':
		return "TAB", nil, true
	}
	return "", nil, false
}
