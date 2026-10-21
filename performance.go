package main

import (
	"fmt"
	"sync"
	"time"
)

type KeyAction struct {
	AtMS     int64  `json:"atMs"`
	ScanCode uint16 `json:"scanCode"`
	Down     bool   `json:"down"`
}

type performanceManager struct {
	mu        sync.Mutex
	actions   []KeyAction
	processes []string
	cancel    chan struct{}
	done      chan struct{}
}

var performance = &performanceManager{}

func (a *AppService) PrepareKeySequence(actions []KeyAction, allowedProcesses []string) error {
	if len(actions) > 1_000_000 || len(allowedProcesses) == 0 {
		return fmt.Errorf("invalid key sequence")
	}
	var previous int64
	for _, action := range actions {
		if action.AtMS < previous || action.AtMS > int64((4*time.Hour)/time.Millisecond) || action.ScanCode == 0 {
			return fmt.Errorf("invalid key action")
		}
		previous = action.AtMS
	}
	performance.stop()
	performance.mu.Lock()
	performance.actions = append([]KeyAction(nil), actions...)
	performance.processes = append([]string(nil), allowedProcesses...)
	performance.mu.Unlock()
	return nil
}

func (a *AppService) StartAutoPlay() error {
	performance.mu.Lock()
	if performance.cancel != nil {
		performance.mu.Unlock()
		return fmt.Errorf("auto play is already running")
	}
	actions := append([]KeyAction(nil), performance.actions...)
	processes := append([]string(nil), performance.processes...)
	if len(actions) == 0 {
		performance.mu.Unlock()
		return fmt.Errorf("no prepared key sequence")
	}
	cancel := make(chan struct{})
	performance.cancel = cancel
	performance.done = make(chan struct{})
	performance.mu.Unlock()
	emitPerformanceEvent("performance:started", time.Now())

	go performance.run(actions, processes, cancel)
	return nil
}

func (a *AppService) PauseAutoPlay() { performance.stop() }
func (a *AppService) StopAutoPlay()  { performance.stop() }

func (a *AppService) SetGamePracticeMonitoring(enabled bool, allowedProcesses []string) error {
	return platformSetGamePracticeMonitoring(enabled, allowedProcesses)
}

func (a *AppService) RegisterPerformanceHotkeys() error {
	return platformRegisterPerformanceHotkeys()
}

func (m *performanceManager) stop() {
	m.mu.Lock()
	done := m.done
	wasRunning := m.cancel != nil
	if m.cancel != nil {
		select {
		case <-m.cancel:
		default:
			close(m.cancel)
		}
	}
	m.mu.Unlock()
	if done != nil {
		<-done
	}
	platformReleaseAllKeys()
	if wasRunning {
		emitPerformanceEvent("performance:stopped", "stopped")
	}
}

func (m *performanceManager) run(actions []KeyAction, processes []string, cancel chan struct{}) {
	m.mu.Lock()
	done := m.done
	m.mu.Unlock()
	defer func() {
		platformReleaseAllKeys()
		m.mu.Lock()
		if m.cancel == cancel {
			m.cancel = nil
		}
		m.done = nil
		m.mu.Unlock()
		close(done)
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	started := time.Now()
	for _, action := range actions {
		wait := time.Until(started.Add(time.Duration(action.AtMS) * time.Millisecond))
		if wait > 0 {
			timer := time.NewTimer(wait)
		waiting:
			for {
				select {
				case <-cancel:
					timer.Stop()
					return
				case <-timer.C:
					break waiting
				case <-ticker.C:
					if !platformForegroundAllowed(processes) {
						timer.Stop()
						emitPerformanceEvent("performance:stopped", "game-not-foreground")
						return
					}
				}
			}
		}
		select {
		case <-cancel:
			return
		default:
		}
		if !platformForegroundAllowed(processes) {
			emitPerformanceEvent("performance:stopped", "game-not-foreground")
			return
		}
		if err := platformSendKey(action.ScanCode, action.Down); err != nil {
			emitPerformanceEvent("performance:error", err.Error())
			return
		}
	}
	emitPerformanceEvent("performance:stopped", "complete")
}

type performanceEvent struct {
	Name string
	Data any
}

var performanceEvents = make(chan performanceEvent, 4096)

func emitPerformanceEvent(name string, data any) {
	select {
	case performanceEvents <- performanceEvent{name, data}:
	default:
	}
}
