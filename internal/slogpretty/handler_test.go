// Copyright 2022 Sylvain Müller. All rights reserved.
// Mount of this source code is governed by a Apache-2.0 license that can be found
// at https://github.com/fox-toolkit/fox/blob/master/LICENSE.txt.

package slogpretty

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_Handle(t *testing.T) {
	bufWo := bytes.NewBuffer(nil)
	bufWe := bytes.NewBuffer(nil)

	h := &Handler{
		We:  &lockedWriter{w: bufWe},
		Wo:  &lockedWriter{w: bufWo},
		Lvl: slog.LevelDebug,
	}

	record := slog.Record{
		Time:    time.Date(2024, 06, 26, 0, 0, 0, 0, time.UTC),
		Message: "::1",
		Level:   slog.LevelDebug,
	}
	record.Add("method", http.MethodGet)
	record.Add("status", http.StatusOK)
	record.Add("latency", 2*time.Second)
	record.Add("location", "../foo")
	record.Add(slog.Group("foo", slog.String("bar", "bar")))
	require.NoError(t, h.Handle(context.Background(), record))
	record.Level = slog.LevelInfo
	require.NoError(t, h.Handle(context.Background(), record))
	record.Level = slog.LevelWarn
	require.NoError(t, h.Handle(context.Background(), record))
	record.Level = slog.LevelError
	require.NoError(t, h.Handle(context.Background(), record))
	record.Message = "unknown"
	require.NoError(t, h.Handle(context.Background(), record))
}

func TestAppendSafe(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: "/home", want: "/home"},
		{in: "bad input: x", want: "bad input: x"},
		{in: "/caf%C3%A9", want: "/caf%C3%A9"},
		{in: "café 日本 🦊", want: "café 日本 🦊"},
		{in: "/home\n[FOX] fake", want: `"/home\n[FOX] fake"`},
		{in: "a\x1b[2J", want: `"a\x1b[2J"`},
		{in: "a\tb", want: `"a\tb"`},
		{in: "a\x7fb", want: `"a\x7fb"`},
		{in: "a\u009b2J", want: `"a\u009b2J"`},
		{in: "a\u202eb", want: `"a\u202eb"`},
		{in: "a\xffb", want: `"a\xffb"`},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, string(appendSafe(nil, tc.in)))
		})
	}
}
