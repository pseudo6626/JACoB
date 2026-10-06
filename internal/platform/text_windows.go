//go:build windows

package platform

import (
	"fmt"
	"time"
	"unicode/utf16"
)

const mapvkVkToVscEx = 4

var (
	procGetKeyboardLayout = user32.NewProc("GetKeyboardLayout")
	procVkKeyScanExW      = user32.NewProc("VkKeyScanExW")
	procMapVirtualKeyExW  = user32.NewProc("MapVirtualKeyExW")
)

func typeTextNative(d InputDriver, text string, interval time.Duration) (bool, error) {
	if _, ok := d.(*windowsInput); !ok {
		return false, nil
	}
	hwnd, _, err := findEliteWindow()
	if err != nil || hwnd == 0 {
		if err != nil {
			return true, err
		}
		return true, fmt.Errorf("Elite Dangerous window was not found")
	}
	threadID := windowThreadID(hwnd)
	layout, _, _ := procGetKeyboardLayout.Call(uintptr(threadID))

	for _, r := range text {
		switch r {
		case '\r':
			continue
		case '\n':
			if err := d.TapKey("ENTER"); err != nil {
				return true, err
			}
		case '\t':
			if err := d.TapKey("TAB"); err != nil {
				return true, err
			}
		default:
			token, mods, ok := windowsLayoutRune(r, layout)
			if ok {
				if err := d.TapChord(token, mods); err != nil {
					return true, err
				}
			} else if err := sendWindowsUnicodeRune(r); err != nil {
				return true, fmt.Errorf("type Unicode character %q: %w", r, err)
			}
		}
		if interval > 0 {
			time.Sleep(interval)
		}
	}
	return true, nil
}

func windowsLayoutRune(r rune, layout uintptr) (string, []string, bool) {
	if r < 0 || r > 0xffff || (r >= 0xd800 && r <= 0xdfff) {
		return "", nil, false
	}
	raw, _, _ := procVkKeyScanExW.Call(uintptr(uint16(r)), layout)
	mapped := uint16(raw & 0xffff)
	if mapped == 0xffff {
		return "", nil, false
	}
	vk := uint16(mapped & 0xff)
	shiftState := byte(mapped >> 8)
	if shiftState&0x08 != 0 { // layout-specific Hankaku state; Unicode fallback is safer
		return "", nil, false
	}
	scanRaw, _, _ := procMapVirtualKeyExW.Call(uintptr(vk), mapvkVkToVscEx, layout)
	if scanRaw == 0 {
		return "", nil, false
	}
	scan := uint32(scanRaw & 0xff)
	prefix := byte((scanRaw >> 8) & 0xff)
	extended := prefix == 0xe0
	token := FormatPhysicalKeyToken(scan, extended)
	if token == "" {
		return "", nil, false
	}
	mods := []string{}
	if shiftState&0x01 != 0 {
		mods = append(mods, "SHIFT")
	}
	// Windows reports AltGr layouts as Ctrl+Alt. Right Alt is the physical
	// AltGr key and lets the active layout perform its normal synthesis.
	if shiftState&0x06 == 0x06 {
		mods = append(mods, "RIGHTALT")
	} else {
		if shiftState&0x02 != 0 {
			mods = append(mods, "CTRL")
		}
		if shiftState&0x04 != 0 {
			mods = append(mods, "ALT")
		}
	}
	return token, mods, true
}

func sendWindowsUnicodeRune(r rune) error {
	units := utf16.Encode([]rune{r})
	if len(units) == 0 {
		return nil
	}
	events := make([]winInput, 0, len(units)*2)
	for _, unit := range units {
		events = append(events, makeWindowsUnicodeEvent(unit, false), makeWindowsUnicodeEvent(unit, true))
	}
	if err := sendInputBatch(events); err != nil {
		return err
	}
	return nil
}
