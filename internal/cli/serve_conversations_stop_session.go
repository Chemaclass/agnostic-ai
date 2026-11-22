package cli

import (
	"errors"
	"net/http"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

func (h *conversationsHandler) stopSession(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	if h.refuseUnproven(w, r, "Stopping a session", "Nothing was stopped.", "stop sessions") {
		return
	}
	var req conversationStopShellRequest
	const usage = "send {tool, id, pid, procStart}"
	if !readConversationRequest(w, r, usage, &req) {
		return
	}
	if strings.TrimSpace(req.ID) == "" || req.PID < 2 || strings.TrimSpace(req.ProcStart) == "" {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: usage})
		return
	}
	reply, err := h.scanner.StopSession(r.Context(), req.Tool, req.ID, req.ProcessRef, h.scanOptions(""))
	switch {
	case err == nil:
		writeEditJSON(w, http.StatusOK, reply)
	case errors.Is(err, conversations.ErrSessionRefused), errors.Is(err, conversations.ErrSessionStopping):
		writeEditJSON(w, http.StatusConflict, editError{Error: err.Error()})
	default:
		writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
	}
}
