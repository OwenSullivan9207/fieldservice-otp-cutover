package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/infrai-examples/fieldservice-otp-cutover/internal/fieldservice"
	"github.com/infrai-examples/fieldservice-otp-cutover/internal/infrai"
)

type server struct {
	sms    *infrai.Client
	orders *fieldservice.Store
}

type otpInput struct {
	OrderID  string `json:"order_id"`
	Phone    string `json:"phone"`
	Code     string `json:"code"`
	RequestID string `json:"request_id"`
}

type photoInput struct {
	ObjectKey string `json:"object_key"`
}

type followUpInput struct {
	Note string `json:"note"`
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	s := &server{
		sms: infrai.NewSMSClient(key),
		orders: fieldservice.NewStore(fieldservice.WorkOrder{
			ID: "WO-1042", TechnicianID: "tech-7", Phone: "+14155550100", Status: fieldservice.StatusAssigned,
		}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/code", s.requestCode)
	mux.HandleFunc("POST /login/verify", s.verifyCode)
	mux.HandleFunc("POST /work-orders/{id}/photos", s.addPhoto)
	mux.HandleFunc("POST /work-orders/{id}/follow-up", s.setFollowUp)
	mux.HandleFunc("GET /work-orders/{id}", s.getOrder)
	log.Printf("dispatch login listening on %s", listenAddress())
	log.Fatal(http.ListenAndServe(listenAddress(), mux))
}

func listenAddress() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}

func (s *server) requestCode(w http.ResponseWriter, r *http.Request) {
	var in otpInput
	if !decode(w, r, &in) {
		return
	}
	order, ok := s.orders.Get(in.OrderID)
	if !ok || order.Phone != in.Phone || in.RequestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work order, phone, or request_id"})
		return
	}
	if err := s.sms.RequestOTP(r.Context(), in.Phone, in.RequestID); err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "code_sent", "order_id": in.OrderID})
}

func (s *server) verifyCode(w http.ResponseWriter, r *http.Request) {
	var in otpInput
	if !decode(w, r, &in) {
		return
	}
	if in.Code == "" || in.RequestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code and request_id are required"})
		return
	}
	order, err := s.orders.DispatchAfterVerification(r.Context(), s.sms, in.OrderID, in.Phone, in.Code, in.RequestID)
	if err != nil {
		if errors.Is(err, fieldservice.ErrOrderNotFound) || errors.Is(err, fieldservice.ErrPhoneMismatch) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *server) addPhoto(w http.ResponseWriter, r *http.Request) {
	var in photoInput
	if !decode(w, r, &in) || strings.TrimSpace(in.ObjectKey) == "" {
		return
	}
	order, err := s.orders.AddPhoto(r.PathValue("id"), fieldservice.Photo{ObjectKey: in.ObjectKey, CapturedAt: time.Now().UTC()})
	writeOrder(w, order, err)
}

func (s *server) setFollowUp(w http.ResponseWriter, r *http.Request) {
	var in followUpInput
	if !decode(w, r, &in) || strings.TrimSpace(in.Note) == "" {
		return
	}
	order, err := s.orders.SetFollowUp(r.PathValue("id"), fieldservice.FollowUp{Note: in.Note, CreatedAt: time.Now().UTC()})
	writeOrder(w, order, err)
}

func (s *server) getOrder(w http.ResponseWriter, r *http.Request) {
	order, ok := s.orders.Get(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fieldservice.ErrOrderNotFound.Error()})
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func writeOrder(w http.ResponseWriter, order fieldservice.WorkOrder, err error) {
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func writeUpstreamError(w http.ResponseWriter, err error) {
	var apiErr *infrai.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Message})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "SMS service request failed"})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
