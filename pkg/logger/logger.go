package logger

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"

	"github.com/fatih/color"
)

var (
	l               *slog.Logger
	DefautlLogLevel = slog.LevelWarn
)

func init() {
	l = slog.New(newLevelHandler(
		slog.LevelWarn,
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}),
	))
}

func Level(level slog.Level) {
	l = slog.New(newLevelHandler(
		level,
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}),
	))
}

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

func Debug(msg string, args ...any) {
	l.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	l.Info(msg, args...)
}
func Warn(msg string, args ...any) {
	l.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	l.Error(msg, args...)
}
