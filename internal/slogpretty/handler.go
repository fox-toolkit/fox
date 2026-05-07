// The code in this package is derivative of https://gitlab.com/greyxor/slogor.
// Mount of this source code is governed by a MIT license that can be found
// at https://gitlab.com/greyxor/slogor/-/blob/main/LICENSE?ref_type=heads.

package slogpretty

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/fox-toolkit/fox/internal/ansi"
)

const (
	maxBufferSize     = 16 << 10 // 16384
	initialBufferSize = 1024
)

var _ slog.Handler = (*Handler)(nil)

var logBufPool = sync.Pool{
	New: func() any {
		return new(make([]byte, 0, initialBufferSize))
	},
}

var (
	DefaultHandler = &Handler{
		We:  &lockedWriter{w: os.Stderr},
		Wo:  &lockedWriter{w: os.Stdout},
		Lvl: slog.LevelDebug,
	}
	timeFormat = fmt.Sprintf("%s %s", time.DateOnly, time.TimeOnly)
)

func freeBuf(b *[]byte) {
	if cap(*b) <= maxBufferSize {
		*b = (*b)[:0]
		logBufPool.Put(b)
	}
}

type Handler struct {
	We          io.Writer
	Wo          io.Writer
	Lvl         slog.Leveler
	groupPrefix string
	attrs       []slog.Attr
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.Lvl.Level()
}

func (h *Handler) Handle(_ context.Context, record slog.Record) error {
	bufp := logBufPool.Get().(*[]byte)
	buf := *bufp

	defer func() {
		*bufp = buf
		freeBuf(bufp)
	}()

	buf = append(buf, "[FOX] "...)

	if !record.Time.IsZero() {
		buf = append(buf, ansi.Faint...)
		buf = record.Time.AppendFormat(buf, timeFormat)
		buf = append(buf, ansi.NormalIntensity...)
		buf = append(buf, ' ')
	}

	// Write level with appropriate formatting and color.
	// Also append right padding depending on the log level.
	buf = append(buf, "| "...)
	switch record.Level {
	case slog.LevelInfo:
		buf = append(buf, ansi.FgGreen...)
		buf = append(buf, record.Level.String()...)
		buf = append(buf, ' ')
	case slog.LevelError:
		buf = append(buf, ansi.FgRed...)
		buf = append(buf, record.Level.String()...)
	case slog.LevelWarn:
		buf = append(buf, ansi.FgYellow...)
		buf = append(buf, record.Level.String()...)
		buf = append(buf, ' ')
	case slog.LevelDebug:
		buf = append(buf, ansi.FgMagenta...)
		buf = append(buf, record.Level.String()...)
	}

	buf = append(buf, ansi.Reset...)
	buf = append(buf, " | "...)
	// Write the log message.
	if record.Message == "unknown" {
		// special case if the ip cannot be found using the ClientIPResolver
		buf = append(buf, ansi.FgRed...)
		buf = append(buf, record.Message...)
		buf = append(buf, ansi.Reset...)
	} else {
		buf = append(buf, record.Message...)
	}
	buf = append(buf, " | "...)

	for i := range h.attrs {
		buf = appendAttr(record.Level, buf, "", h.attrs[i])
	}

	if record.NumAttrs() > 0 {
		prefix := h.groupPrefix
		record.Attrs(func(attr slog.Attr) bool {
			buf = appendAttr(record.Level, buf, prefix, attr)
			return true
		})
	}

	// Replace the latest space by an EOL.
	buf[len(buf)-1] = '\n'

	if record.Level >= slog.LevelError {
		if _, err := h.We.Write(buf); err != nil {
			return fmt.Errorf("failed to write buffer: %w", err)
		}
	} else {
		if _, err := h.Wo.Write(buf); err != nil {
			return fmt.Errorf("failed to write buffer: %w", err)
		}
	}

	return nil
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	merged := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	for _, attr := range attrs {
		if h.groupPrefix != "" {
			attr.Key = h.groupPrefix + attr.Key
		}
		merged = append(merged, attr)
	}

	return &Handler{
		We:          h.We,
		Wo:          h.Wo,
		Lvl:         h.Lvl,
		attrs:       merged,
		groupPrefix: h.groupPrefix,
	}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &Handler{
		We:          h.We,
		Wo:          h.Wo,
		Lvl:         h.Lvl,
		attrs:       h.attrs,
		groupPrefix: h.groupPrefix + name + ".",
	}
}

func appendAttr(level slog.Level, buf []byte, keyPrefix string, attr slog.Attr) []byte {
	attr.Value = attr.Value.Resolve()

	if attr.Equal(slog.Attr{}) {
		return buf
	}

	buf = append(buf, ansi.Faint...)
	buf = append(buf, ansi.Bold...)

	if keyPrefix != "" {
		buf = append(buf, keyPrefix...)
	}
	buf = append(buf, attr.Key...)
	buf = append(buf, '=')
	buf = append(buf, ansi.NormalIntensity...)

	switch attr.Key {
	case "method":
		buf = append(buf, ansi.BgBlue...)
		buf = append(buf, ' ')
		buf = appendValue(buf, attr.Value)
		buf = append(buf, ' ')
	case "status":
		buf = append(buf, levelColor(level)...)
		buf = append(buf, ' ')
		buf = appendValue(buf, attr.Value)
		buf = append(buf, ' ')
	case "location":
		buf = append(buf, ansi.FgYellow...)
		buf = appendValue(buf, attr.Value)
	case "latency":
		dt := roundLatency(attr.Value.Duration())
		buf = append(buf, latencyColor(dt)...)
		buf = append(buf, dt.String()...)
	case "error":
		buf = append(buf, ansi.FgRed...)
		buf = appendValue(buf, attr.Value)
	default:
		buf = append(buf, ansi.FgCyan...)
		buf = appendValue(buf, attr.Value)
	}

	buf = append(buf, ansi.Reset...)
	buf = append(buf, ' ')

	return buf
}

func appendValue(buf []byte, v slog.Value) []byte {
	switch v.Kind() {
	case slog.KindString:
		return append(buf, v.String()...)
	case slog.KindInt64:
		return strconv.AppendInt(buf, v.Int64(), 10)
	case slog.KindUint64:
		return strconv.AppendUint(buf, v.Uint64(), 10)
	case slog.KindBool:
		return strconv.AppendBool(buf, v.Bool())
	case slog.KindFloat64:
		return strconv.AppendFloat(buf, v.Float64(), 'g', -1, 64)
	case slog.KindDuration:
		return append(buf, v.Duration().String()...)
	case slog.KindTime:
		return v.Time().AppendFormat(buf, time.RFC3339Nano)
	default:
		return append(buf, v.String()...)
	}
}

type lockedWriter struct {
	w io.Writer
	sync.Mutex
}

func (w *lockedWriter) Write(p []byte) (n int, err error) {
	w.Lock()
	n, err = w.w.Write(p)
	w.Unlock()
	return
}

func levelColor(level slog.Level) string {
	switch level {
	case slog.LevelInfo:
		return ansi.BgBlue
	case slog.LevelWarn:
		return ansi.BgYellow
	case slog.LevelError:
		return ansi.BgRed
	default:
		return ansi.BgMagenta
	}
}

func latencyColor(d time.Duration) string {
	if d < 100*time.Millisecond {
		return ansi.FgGreen
	}
	if d < 500*time.Millisecond {
		return ansi.FgYellow
	}
	return ansi.FgRed
}

func roundLatency(d time.Duration) time.Duration {
	switch {
	case d < 1*time.Microsecond:
		return d.Round(100 * time.Nanosecond)
	case d < 1*time.Millisecond:
		return d.Round(10 * time.Microsecond)
	case d < 10*time.Millisecond:
		return d.Round(100 * time.Microsecond)
	case d < 100*time.Millisecond:
		return d.Round(1 * time.Millisecond)
	case d < 1*time.Second:
		return d.Round(10 * time.Millisecond)
	case d < 10*time.Second:
		return d.Round(100 * time.Millisecond)
	default:
		return d.Round(1 * time.Second)
	}
}
