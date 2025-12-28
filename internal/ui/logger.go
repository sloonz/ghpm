package ui

import (
	"fmt"
	"io"
)

type Level int

const (
	LevelSilent Level = iota
	LevelNormal
	LevelVerbose
)

type Logger struct {
	Level  Level
	Writer io.Writer
}

func NewLogger(level Level, w io.Writer) Logger {
	return Logger{Level: level, Writer: w}
}

func (l Logger) Infof(format string, args ...any) {
	if l.Level < LevelNormal || l.Writer == nil {
		return
	}
	fmt.Fprintf(l.Writer, format+"\n", args...)
}

func (l Logger) Verbosef(format string, args ...any) {
	if l.Level < LevelVerbose || l.Writer == nil {
		return
	}
	fmt.Fprintf(l.Writer, format+"\n", args...)
}
