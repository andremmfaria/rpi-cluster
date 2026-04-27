package printer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfoDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { Info("hello") })
}

func TestSuccessDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { Success("done") })
}

func TestWarnDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { Warn("careful") })
}

func TestErrorDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { Error("boom") })
}

func TestConfirmFromYes(t *testing.T) {
	assert.True(t, ConfirmFrom(strings.NewReader("yes\n"), "Type 'yes' to confirm"))
}

func TestConfirmFromNo(t *testing.T) {
	assert.False(t, ConfirmFrom(strings.NewReader("no\n"), "Type 'yes' to confirm"))
}

func TestConfirmFromEmpty(t *testing.T) {
	assert.False(t, ConfirmFrom(strings.NewReader("\n"), "Type 'yes' to confirm"))
}

func TestConfirmFromShutdown(t *testing.T) {
	assert.True(t, ConfirmFrom(strings.NewReader("shutdown\n"), "Type 'shutdown' to confirm"))
}

func TestConfirmFromShutdownWrongWord(t *testing.T) {
	assert.False(t, ConfirmFrom(strings.NewReader("yes\n"), "Type 'shutdown' to confirm"))
}
