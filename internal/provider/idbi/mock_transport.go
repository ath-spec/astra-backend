package idbi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MockTransport intercepts HTTP requests to the IDBI API and serves static
// JSON responses from the testdata directory based on the URL path.
type MockTransport struct {
	BaseDir string
}

func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// e.g. path: "/Development/performAccountEnquirytest"
	// filename: "Development_performAccountEnquirytest.json"
	filename := strings.TrimPrefix(req.URL.Path, "/")
	filename = strings.ReplaceAll(filename, "/", "_") + ".json"
	
	// Optional: some queries have samples, e.g. "__Sample1". For a simple mock, 
	// if the exact file doesn't exist, we can just try to find any file that starts with the base name.
	path := filepath.Join(m.BaseDir, filename)
	
	data, err := os.ReadFile(path)
	if err != nil {
		// Try to find a fallback if exact match doesn't exist (e.g. for dynamic test data)
		files, _ := filepath.Glob(strings.TrimSuffix(path, ".json") + "*.json")
		if len(files) > 0 {
			data, err = os.ReadFile(files[0])
		}
		
		if err != nil {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader([]byte(fmt.Sprintf(`{"message":"mock file not found for %s"}`, req.URL.Path)))),
				Header:     make(http.Header),
			}, nil
		}
	}

	// Unwrap Postman-style captures ({"request": {...}, "response": {...}})
	// if present, otherwise serve the raw data.
	var capture struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(data, &capture); err == nil && len(capture.Response) > 0 {
		data = capture.Response
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(data)),
		Header:     make(http.Header),
	}, nil
}

