package cli

import (
	"errors"
	"net/http"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

type conversationStopShellRequest struct {
	conversationRef
	conversations.ProcessRef
}

func (h *conversationsHandler) stopShell(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	if h.refuseUnproven(w, r, "Stopping a shell", "Nothing was stopped.", "stop shells") {
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
	reply, err := h.scanner.StopShell(r.Context(), req.Tool, req.ID, req.ProcessRef, h.scanOptions(""))
	switch {
	case err == nil:
		writeEditJSON(w, http.StatusOK, reply)
	case errors.Is(err, conversations.ErrShellRefused):
		writeEditJSON(w, http.StatusConflict, editError{Error: err.Error()})
	default:
		writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
	}
}
