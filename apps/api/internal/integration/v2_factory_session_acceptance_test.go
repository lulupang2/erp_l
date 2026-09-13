package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestV2FactoryRejectsWrongOrEndedWorkSession(t *testing.T) {
	h := newAcc(t)
	first := h.order(t, 1)
	firstSession := h.session(t, first)
	second := h.order(t, 1)
	secondSession := h.session(t, second)

	wrong := h.doc(t, "operator", map[string]any{
		"kind":            "production",
		"order_id":        second,
		"work_session_id": firstSession,
		"quantity":        1,
	})
	accCode(t, h.post(t, "operator", wrong, http.StatusForbidden), "FORBIDDEN")

	h.expect(t, "operator", http.MethodPost, "/api/v2/work-sessions/"+secondSession.String()+"/end", nil, uuid.Nil, http.StatusOK)
	ended := h.doc(t, "operator", map[string]any{
		"kind":            "production",
		"order_id":        second,
		"work_session_id": secondSession,
		"quantity":        1,
	})
	accCode(t, h.post(t, "operator", ended, http.StatusConflict), "WORK_SESSION_NOT_ACTIVE")
}
