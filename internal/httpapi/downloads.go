package httpapi

import (
	"io/fs"
	"net/http"
	"strings"
)

const downloadsPrefix = "/downloads/"

var agentDownloadNames = map[string]struct{}{
	"tinyprobe-agent-linux-amd64": {},
	"tinyprobe-agent-linux-arm64": {},
}

func AgentDownloadHandler(files fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(r.URL.Path, downloadsPrefix)
		if _, allowed := agentDownloadNames[name]; !allowed || files == nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		http.ServeFileFS(w, r, files, name)
	})
}
