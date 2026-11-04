package groq

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/whiteowltech/owli/backend/internal/ports"
)

func TestTranscribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("petición inesperada: %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("model"); got != "whisper-large-v3-turbo" {
			t.Errorf("model = %q", got)
		}
		f, h, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(f)
		if string(data) != "audio" || h.Filename != "audio.webm" {
			t.Errorf("archivo = %q (%s)", data, h.Filename)
		}
		_, _ = w.Write([]byte(`{"text":"  dos empanadas de carne ","duration":3.2}`))
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", Model: "whisper-large-v3-turbo", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Transcribe(context.Background(), ports.Audio{Data: []byte("audio"), MIMEType: "audio/webm;codecs=opus"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "dos empanadas de carne" || got.Seconds != 3.2 {
		t.Errorf("transcripción = %+v", got)
	}
}

func TestTranscribeErrors(t *testing.T) {
	tests := []struct {
		status    int
		retryable bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusBadGateway, true},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
	}
	for _, tt := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"x"}`, tt.status)
		}))
		c, _ := New(Config{APIKey: "k", Model: "m", BaseURL: srv.URL})
		_, err := c.Transcribe(context.Background(), ports.Audio{Data: []byte("a"), MIMEType: "audio/ogg"})
		srv.Close()
		if err == nil || ports.IsRetryable(err) != tt.retryable {
			t.Errorf("HTTP %d: err = %v, reintentable = %v", tt.status, err, ports.IsRetryable(err))
		}
	}
	c, _ := New(Config{APIKey: "k", Model: "m"})
	if _, err := c.Transcribe(context.Background(), ports.Audio{}); err == nil || errors.Is(err, ports.ErrProviderUnavailable) {
		t.Errorf("audio vacío: %v", err)
	}
}
