package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"

	"github.com/fatih/color"
)

type levelHandler struct {
	level   slog.Leveler
	handler slog.Handler
	l       *log.Logger
}

func (h *levelHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *levelHandler) Disable() {
	h.l.SetOutput(io.Discard)
}

func (h *levelHandler) Handle(ctx context.Context, r slog.Record) error {
	level := r.Level.String() + ":"

	switch r.Level {
	case slog.LevelDebug:
		level = color.MagentaString(level)
	case slog.LevelInfo:
		level = color.BlueString(level)
	case slog.LevelWarn:
		level = color.YellowString(level)
	case slog.LevelError:
		level = color.RedString(level)
	}

	fields := ""
	r.Attrs(func(a slog.Attr) bool {
		fields += a.Key + "=" + fmt.Sprintf("%v ", a.Value)
		return true
	})

	timeStr := r.Time.Format("[15:05:05.000]")
	msg := color.CyanString(r.Message)

	h.l.Println(timeStr, level, msg, color.WhiteString(fields))

	return nil
}

func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return newLevelHandler(h.level, h.handler.WithAttrs(attrs))
}

func (h *levelHandler) WithGroup(name string) slog.Handler {
	return newLevelHandler(h.level, h.handler.WithGroup(name))
}

func (h *levelHandler) Handler() slog.Handler {
	return h.handler
}

func newLevelHandler(level slog.Leveler, h slog.Handler) *levelHandler {
	if lh, ok := h.(*levelHandler); ok {
		h = lh.Handler()
	}
	return &levelHandler{level, h, log.New(os.Stdout, "", 0)}
}
