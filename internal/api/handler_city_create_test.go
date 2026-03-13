package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCityCreate_ValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		body   any
		status int
		code   string
	}{
		{
			name:   "missing dir",
			body:   map[string]string{"provider": "claude"},
			status: http.StatusBadRequest,
			code:   "invalid",
		},
		{
			name:   "missing provider",
			body:   map[string]string{"dir": "/tmp/test-city"},
			status: http.StatusBadRequest,
			code:   "invalid",
		},
		{
			name:   "unknown provider",
			body:   map[string]string{"dir": "/tmp/test-city", "provider": "unknown-agent"},
			status: http.StatusBadRequest,
			code:   "invalid",
		},
		{
			name:   "unknown bootstrap profile",
			body:   map[string]string{"dir": "/tmp/test-city", "provider": "claude", "bootstrap_profile": "invalid-profile"},
			status: http.StatusBadRequest,
			code:   "invalid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/v0/city", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handleCityCreate(w, req)

			if w.Code != tc.status {
				t.Errorf("status = %d, want %d (body: %s)", w.Code, tc.status, w.Body.String())
			}

			var resp Error
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("invalid JSON response: %v", err)
			}
			if resp.Code != tc.code {
				t.Errorf("code = %q, want %q", resp.Code, tc.code)
			}
		})
	}
}
