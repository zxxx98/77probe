package httpapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

const downloadsPrefix = "/downloads/"

var agentDownloadNames = map[string]struct{}{
	"tinyprobe-agent-linux-amd64": {},
	"tinyprobe-agent-linux-arm64": {},
}

func AgentDownloadHandler(files fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, downloadsPrefix)
		if _, allowed := agentDownloadNames[name]; !allowed || files == nil {
			http.NotFound(w, r)
			return
		}

		body, err := fs.ReadFile(files, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
	})
}
