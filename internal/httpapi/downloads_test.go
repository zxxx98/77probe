package httpapi_test

import (
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"probe.local/monitor/internal/httpapi"
)

type trackingFS struct {
	fs.FS
	opens int
}

type streamingFS struct {
	name     string
	data     []byte
	lastFile *streamingFile
}

func (files *streamingFS) Open(name string) (fs.File, error) {
	if name != files.name {
		return nil, fs.ErrNotExist
	}
	files.lastFile = &streamingFile{reader: bytes.NewReader(files.data), size: int64(len(files.data))}
	return files.lastFile, nil
}

type streamingFile struct {
	reader    *bytes.Reader
	size      int64
	bytesRead int
}

func (file *streamingFile) Read(buffer []byte) (int, error) {
	count, err := file.reader.Read(buffer)
	file.bytesRead += count
	return count, err
}

func (file *streamingFile) Seek(offset int64, whence int) (int64, error) {
	return file.reader.Seek(offset, whence)
}

func (*streamingFile) Close() error { return nil }

func (file *streamingFile) Stat() (fs.FileInfo, error) {
	return streamingFileInfo{size: file.size}, nil
}

type streamingFileInfo struct{ size int64 }

func (streamingFileInfo) Name() string       { return "agent" }
func (info streamingFileInfo) Size() int64   { return info.size }
func (streamingFileInfo) Mode() fs.FileMode  { return 0o644 }
func (streamingFileInfo) ModTime() time.Time { return time.Time{} }
func (streamingFileInfo) IsDir() bool        { return false }
func (streamingFileInfo) Sys() any           { return nil }

type abortingResponseWriter struct {
	header http.Header
	wrote  int
}

func (writer *abortingResponseWriter) Header() http.Header {
	return writer.header
}

func (*abortingResponseWriter) WriteHeader(int) {}

func (writer *abortingResponseWriter) Write(body []byte) (int, error) {
	count := min(len(body), 1024)
	writer.wrote += count
	return count, errors.New("client disconnected")
}

func (files *trackingFS) Open(name string) (fs.File, error) {
	files.opens++
	return files.FS.Open(name)
}

func TestAgentDownloadHandlerServesOnlyAllowlistedBinariesAsAttachments(t *testing.T) {
	files := fstest.MapFS{
		"tinyprobe-agent-linux-amd64": {Data: []byte("amd64-binary")},
		"tinyprobe-agent-linux-arm64": {Data: []byte("arm64-binary")},
	}

	for _, test := range []struct {
		name string
		body string
	}{
		{name: "tinyprobe-agent-linux-amd64", body: "amd64-binary"},
		{name: "tinyprobe-agent-linux-arm64", body: "arm64-binary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/downloads/"+test.name, nil)
			rec := httptest.NewRecorder()

			httpapi.AgentDownloadHandler(files).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
			}
			if rec.Body.String() != test.body {
				t.Fatalf("body=%q, want %q", rec.Body.String(), test.body)
			}
			wantDisposition := `attachment; filename="` + test.name + `"`
			if got := rec.Header().Get("Content-Disposition"); got != wantDisposition {
				t.Fatalf("Content-Disposition=%q, want %q", got, wantDisposition)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
				t.Fatalf("Content-Type=%q, want application/octet-stream", got)
			}
		})
	}
}

func TestAgentDownloadHandlerSupportsHeadAndRanges(t *testing.T) {
	const name = "tinyprobe-agent-linux-amd64"
	files := fstest.MapFS{name: {Data: []byte("0123456789")}}
	handler := httpapi.AgentDownloadHandler(files)

	t.Run("HEAD", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodHead, "/downloads/"+name, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("HEAD body length=%d, want 0", rec.Body.Len())
		}
		if got := rec.Header().Get("Content-Length"); got != "10" {
			t.Fatalf("Content-Length=%q, want 10", got)
		}
	})

	t.Run("range", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/downloads/"+name, nil)
		req.Header.Set("Range", "bytes=2-5")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusPartialContent {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Range"); got != "bytes 2-5/10" {
			t.Fatalf("Content-Range=%q, want bytes 2-5/10", got)
		}
		if got := rec.Body.String(); got != "2345" {
			t.Fatalf("body=%q, want %q", got, "2345")
		}
	})
}

func TestAgentDownloadHandlerRejectsUnsupportedMethodsBeforeOpeningFile(t *testing.T) {
	const name = "tinyprobe-agent-linux-amd64"
	files := &trackingFS{FS: fstest.MapFS{name: {Data: []byte("binary")}}}
	req := httptest.NewRequest(http.MethodPost, "/downloads/"+name, nil)
	rec := httptest.NewRecorder()

	httpapi.AgentDownloadHandler(files).ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("Allow=%q, want GET, HEAD", got)
	}
	if files.opens != 0 {
		t.Fatalf("filesystem opened %d times, want 0", files.opens)
	}
}

func TestAgentDownloadHandlerServesProductionSizedBinary(t *testing.T) {
	const name = "tinyprobe-agent-linux-arm64"
	body := bytes.Repeat([]byte("tinyprobe-agent"), 256*1024)
	files := fstest.MapFS{name: {Data: body}}
	req := httptest.NewRequest(http.MethodGet, "/downloads/"+name, nil)
	rec := httptest.NewRecorder()

	httpapi.AgentDownloadHandler(files).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body-prefix=%q", rec.Code, rec.Body.Bytes()[:min(rec.Body.Len(), 100)])
	}
	if !bytes.Equal(rec.Body.Bytes(), body) {
		t.Fatalf("body length=%d, want %d", rec.Body.Len(), len(body))
	}
}

func TestAgentDownloadHandlerStreamsBeforeReadingTheWholeFile(t *testing.T) {
	const name = "tinyprobe-agent-linux-amd64"
	body := bytes.Repeat([]byte("tinyprobe-agent"), 256*1024)
	files := &streamingFS{name: name, data: body}
	writer := &abortingResponseWriter{header: make(http.Header)}
	req := httptest.NewRequest(http.MethodGet, "/downloads/"+name, nil)

	httpapi.AgentDownloadHandler(files).ServeHTTP(writer, req)

	if files.lastFile == nil {
		t.Fatal("download file was not opened")
	}
	if writer.wrote == 0 {
		t.Fatal("response did not begin streaming")
	}
	if files.lastFile.bytesRead >= len(body) {
		t.Fatalf("read %d bytes before client disconnect, want less than %d", files.lastFile.bytesRead, len(body))
	}
}

func TestAgentDownloadHandlerRejectsUnknownNamesWithoutListing(t *testing.T) {
	files := fstest.MapFS{
		"tinyprobe-agent-linux-amd64": {Data: []byte("amd64-binary")},
	}
	for _, path := range []string{
		"/downloads/",
		"/downloads/not-an-agent",
		"/downloads/tinyprobe-agent-linux-amd64/extra",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		httpapi.AgentDownloadHandler(files).ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("path=%q status=%d body=%q", path, rec.Code, rec.Body.String())
		}
	}
}

func TestAgentDownloadRoutesArePublicAndMissingFilesAreSafe404(t *testing.T) {
	files := fstest.MapFS{
		"tinyprobe-agent-linux-amd64": {Data: []byte("amd64-binary")},
	}
	router := httpapi.NewRouter(httpapi.Dependencies{AgentFiles: files})

	public := httptest.NewRecorder()
	router.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/downloads/tinyprobe-agent-linux-amd64", nil))
	if public.Code != http.StatusOK || public.Body.String() != "amd64-binary" {
		t.Fatalf("public download: status=%d body=%q", public.Code, public.Body.String())
	}

	missingRouter := httpapi.NewRouter(httpapi.Dependencies{})
	missing := httptest.NewRecorder()
	missingRouter.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/downloads/tinyprobe-agent-linux-arm64", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing binary: status=%d body=%q", missing.Code, missing.Body.String())
	}
}
