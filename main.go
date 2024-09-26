package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"github.com/hypebeast/go-osc/osc"
	"github.com/lxn/win"
)

// Constants
const (
	WH_MOUSE_LL    = 14
	WM_LBUTTONDOWN = 0x0201
	WM_LBUTTONUP   = 0x0202
)

// Types
type (
	HHOOK   uintptr
	WPARAM  uintptr
	LPARAM  uintptr
	LRESULT uintptr
)

// Windows API functions
var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procSetWindowsHookEx    = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
)

// Global variables
var (
	hookID    HHOOK
	oscClient *osc.Client
	msgChan   chan int32
	quitChan  chan struct{}
)

func main() {
	// Initialize OSC client
	oscClient = osc.NewClient("192.168.1.91", 8001)

	// Initialize channels
	msgChan = make(chan int32, 100) // Buffered channel to prevent blocking
	quitChan = make(chan struct{})

	// Start OSC message sender goroutine
	go oscSender()

	// Set the mouse hook
	if err := setMouseHook(); err != nil {
		log.Fatalf("Error setting mouse hook: %v", err)
	}
	defer removeMouseHook()

	// Handle graceful shutdown on interrupt signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// Start the message loop in a separate goroutine
	go messageLoop()

	// Wait for interrupt signal
	<-signalChan
	log.Println("Received interrupt signal, shutting down...")

	// Clean up resources
	removeMouseHook()
	close(msgChan)
	<-quitChan // Wait for oscSender to finish
}

func setMouseHook() error {
	cb := syscall.NewCallback(mouseHook)
	hInstance, _, err := procGetModuleHandle.Call(0)
	if hInstance == 0 {
		return err
	}
	hook, _, err := procSetWindowsHookEx.Call(
		uintptr(WH_MOUSE_LL),
		cb,
		hInstance,
		0,
	)
	if hook == 0 {
		return err
	}
	hookID = HHOOK(hook)
	return nil
}

func removeMouseHook() {
	if hookID != 0 {
		procUnhookWindowsHookEx.Call(uintptr(hookID))
		hookID = 0
	}
}

func messageLoop() {
	var msg win.MSG
	for {
		ret, _, err := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)
		if ret == 0 { // WM_QUIT
			break
		}
		if ret == uintptr(^uint32(0)) { // -1 indicates an error
			log.Printf("GetMessageW error: %v", err)
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func mouseHook(nCode int, wParam WPARAM, lParam LPARAM) LRESULT {
	if nCode >= 0 {
		var state int32 = -1
		switch wParam {
		case WM_LBUTTONDOWN:
			log.Println("Mouse button pressed")
			state = 1
		case WM_LBUTTONUP:
			log.Println("Mouse button released")
			state = 0
		}
		if state != -1 {
			select {
			case msgChan <- state:
			default:
				log.Println("Message channel is full, dropping message")
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(
		uintptr(0),
		uintptr(nCode),
		uintptr(wParam),
		uintptr(lParam),
	)
	return LRESULT(ret)
}

func oscSender() {
	defer close(quitChan)
	for state := range msgChan {
		msg := osc.NewMessage("/mouse")
		msg.Append(state)
		if err := oscClient.Send(msg); err != nil {
			log.Printf("Failed to send OSC message: %v", err)
		} else {
			log.Printf("Sent OSC message: /mouse %d", state)
		}
	}
}
