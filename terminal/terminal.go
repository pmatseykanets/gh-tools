package terminal

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"
)

// PasswordPrompt reads the password from the terminal.
// It resets terminal echo after ^C interrupts.
// Source: https://gist.github.com/jlinoff/e8e26b4ffa38d379c7f1891fd174a6d0
func PasswordPrompt(prompt ...string) (string, error) {
	// Get the initial state of the terminal.
	state, err := terminal.GetState(int(syscall.Stdin))
	if err != nil {
		return "", err
	}

	// Restore the state in the event of an interrupt.
	// See: https://groups.google.com/forum/#!topic/golang-nuts/kTVAbtee9UA
	sig := make(chan os.Signal, 1)
	quit := make(chan struct{})
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sig:
			_ = terminal.Restore(int(syscall.Stdin), state)
			fmt.Println()
			os.Exit(1)
		case <-quit:
			return
		}
	}()

	text := "Password: "
	if len(prompt) > 0 {
		text = prompt[0]
	}
	fmt.Print(text)

	password, err := terminal.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}

	signal.Stop(sig)
	close(quit)

	// Return the password as a string.
	return string(password), nil
}
