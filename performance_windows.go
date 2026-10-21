//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	procSendInput              = user32.NewProc("SendInput")
	procGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procSetWindowsHookEx       = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx    = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx         = user32.NewProc("CallNextHookEx")
	procGetMessage             = user32.NewProc("GetMessageW")
	procPostThreadMessage      = user32.NewProc("PostThreadMessageW")
	procRegisterHotKey         = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey       = user32.NewProc("UnregisterHotKey")
	kernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	procGetCurrentThreadID     = kernel32.NewProc("GetCurrentThreadId")
	pressedScanCodes           = struct {
		sync.Mutex
		values map[uint16]bool
	}{values: map[uint16]bool{}}
)

var keyboardMonitor = struct {
	sync.Mutex
	hook      uintptr
	threadID  uint32
	processes []string
}{}

var hotkeysOnce sync.Once
var hotkeysErr error

type winMessage struct {
	Window  uintptr
	Message uint32
	_       uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	X       int32
	Y       int32
	Private uint32
}

type lowLevelKeyboardInput struct {
	VirtualKey uint32
	ScanCode   uint32
	Flags      uint32
	Time       uint32
	ExtraInfo  uintptr
}

var keyboardHookCallback = windows.NewCallback(func(code int, message uintptr, input *lowLevelKeyboardInput) uintptr {
	if code >= 0 {
		keyboardMonitor.Lock()
		processes := append([]string(nil), keyboardMonitor.processes...)
		keyboardMonitor.Unlock()
		if input.Flags&0x10 == 0 && platformForegroundAllowed(processes) {
			down := message == 0x0100 || message == 0x0104
			up := message == 0x0101 || message == 0x0105
			if down || up {
				emitPerformanceEvent("performance:key", map[string]any{"scanCode": input.ScanCode, "down": down})
			}
		} else if input.Flags&0x10 == 0 && len(processes) > 0 {
			emitPerformanceEvent("performance:focus-lost", nil)
		}
	}
	result, _, _ := procCallNextHookEx.Call(0, uintptr(code), message, uintptr(unsafe.Pointer(input)))
	return result
})

type keyboardInput struct {
	VirtualKey uint16
	ScanCode   uint16
	Flags      uint32
	Time       uint32
	ExtraInfo  uintptr
}

type winInput struct {
	Type uint32
	_    uint32
	Data [32]byte
}

func platformSendKey(scanCode uint16, down bool) error {
	input := winInput{Type: 1}
	keyboard := (*keyboardInput)(unsafe.Pointer(&input.Data[0]))
	keyboard.ScanCode = scanCode
	keyboard.Flags = 0x0008
	if !down {
		keyboard.Flags |= 0x0002
	}
	result, _, callErr := procSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	if result != 1 {
		return fmt.Errorf("SendInput failed: %w", callErr)
	}
	pressedScanCodes.Lock()
	if down {
		pressedScanCodes.values[scanCode] = true
	} else {
		delete(pressedScanCodes.values, scanCode)
	}
	pressedScanCodes.Unlock()
	return nil
}

func platformReleaseAllKeys() {
	pressedScanCodes.Lock()
	keys := make([]uint16, 0, len(pressedScanCodes.values))
	for key := range pressedScanCodes.values {
		keys = append(keys, key)
	}
	pressedScanCodes.Unlock()
	for _, key := range keys {
		_ = platformSendKey(key, false)
	}
}

func platformForegroundAllowed(allowed []string) bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}
	var processID uint32
	procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&processID)))
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, processID)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return false
	}
	name := filepath.Base(windows.UTF16ToString(buffer[:size]))
	for _, candidate := range allowed {
		if strings.EqualFold(name, candidate) {
			return true
		}
	}
	return false
}

func platformSetGamePracticeMonitoring(enabled bool, processes []string) error {
	keyboardMonitor.Lock()
	defer keyboardMonitor.Unlock()
	keyboardMonitor.processes = append([]string(nil), processes...)
	if !enabled {
		if keyboardMonitor.hook != 0 {
			procUnhookWindowsHookEx.Call(keyboardMonitor.hook)
			procPostThreadMessage.Call(uintptr(keyboardMonitor.threadID), 0x0012, 0, 0)
			keyboardMonitor.hook = 0
		}
		return nil
	}
	if len(processes) == 0 {
		return fmt.Errorf("game process whitelist is empty")
	}
	if keyboardMonitor.hook != 0 {
		return nil
	}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		threadID, _, _ := procGetCurrentThreadID.Call()
		hook, _, callErr := procSetWindowsHookEx.Call(13, keyboardHookCallback, 0, 0)
		if hook == 0 {
			ready <- fmt.Errorf("SetWindowsHookEx failed: %w", callErr)
			return
		}
		keyboardMonitor.Lock()
		keyboardMonitor.hook = hook
		keyboardMonitor.threadID = uint32(threadID)
		keyboardMonitor.Unlock()
		ready <- nil
		var message [48]byte
		for {
			value, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&message[0])), 0, 0, 0)
			if int32(value) <= 0 {
				break
			}
		}
		procUnhookWindowsHookEx.Call(hook)
	}()
	keyboardMonitor.Unlock()
	err := <-ready
	keyboardMonitor.Lock()
	return err
}

func platformRegisterPerformanceHotkeys() error {
	hotkeysOnce.Do(func() {
		ready := make(chan error, 1)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			if ok, _, err := procRegisterHotKey.Call(0, 1, 0x0003, 0x77); ok == 0 {
				ready <- fmt.Errorf("register Ctrl+Alt+F8: %w", err)
				return
			}
			if ok, _, err := procRegisterHotKey.Call(0, 2, 0x0003, 0x78); ok == 0 {
				procUnregisterHotKey.Call(0, 1)
				ready <- fmt.Errorf("register Ctrl+Alt+F9: %w", err)
				return
			}
			ready <- nil
			defer procUnregisterHotKey.Call(0, 1)
			defer procUnregisterHotKey.Call(0, 2)
			var message winMessage
			for {
				value, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
				if int32(value) <= 0 {
					return
				}
				if message.Message != 0x0312 {
					continue
				}
				if message.WParam == 2 {
					performance.stop()
					continue
				}
				performance.mu.Lock()
				running := performance.cancel != nil
				performance.mu.Unlock()
				if running {
					performance.stop()
				} else if err := (&AppService{}).StartAutoPlay(); err != nil {
					emitPerformanceEvent("performance:error", err.Error())
				}
			}
		}()
		hotkeysErr = <-ready
	})
	return hotkeysErr
}
