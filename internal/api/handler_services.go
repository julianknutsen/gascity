package api

import (
	"net"
	"net/http"
	"strings"

	"github.com/gastownhall/gascity/internal/workspacesvc"
)

func (s *Server) handleServiceList(w http.ResponseWriter, _ *http.Request) {
	reg := s.state.ServiceRegistry()
	if reg == nil {
		writeListJSON(w, s.latestIndex(), []any{}, 0)
		return
	}
	items := reg.List()
	writeListJSON(w, s.latestIndex(), items, len(items))
}

func (s *Server) handleServiceGet(w http.ResponseWriter, r *http.Request) {
	reg := s.state.ServiceRegistry()
	if reg == nil {
		writeError(w, http.StatusNotFound, "not_found", "service "+r.PathValue("name")+" not found")
		return
	}
	item, ok := reg.Get(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "service "+r.PathValue("name")+" not found")
		return
	}
	writeIndexJSON(w, s.latestIndex(), item)
}

func (s *Server) handleServiceProxy(w http.ResponseWriter, r *http.Request) {
	reg := s.state.ServiceRegistry()
	if reg == nil {
		writeError(w, http.StatusNotFound, "not_found", "service route not found")
		return
	}
	name := serviceNameFromPath(r.URL.Path)
	if name == "" {
		writeError(w, http.StatusNotFound, "not_found", "service route not found")
		return
	}
	status, ok := reg.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "service route not found")
		return
	}
	if !serviceRequestAllowed(w, status, r) {
		return
	}
	if !reg.ServeHTTP(w, r) {
		writeError(w, http.StatusNotFound, "not_found", "service route not found")
	}
}

func serviceNameFromPath(path string) string {
	path = strings.TrimPrefix(path, "/svc/")
	if path == "" || path == "/svc/" {
		return ""
	}
	if i := strings.IndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	return path
}

func serviceRequestAllowed(w http.ResponseWriter, status workspacesvc.Status, r *http.Request) bool {
	if status.PublishMode == "" {
		status.PublishMode = "private"
	}
	if status.PublishMode == "private" {
		if !isLoopbackRemoteAddr(r.RemoteAddr) {
			writeError(w, http.StatusNotFound, "not_found", "service route not found")
			return false
		}
		if isMutationMethod(r.Method) && r.Header.Get("X-GC-Request") == "" {
			writeError(w, http.StatusForbidden, "csrf", "X-GC-Request header required on private service mutation endpoints")
			return false
		}
	}
	return true
}

func isLoopbackRemoteAddr(remoteAddr string) bool {
	if remoteAddr == "" {
		return false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
