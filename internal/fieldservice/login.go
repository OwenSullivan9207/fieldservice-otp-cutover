package fieldservice

import (
	"context"
	"errors"
	"sync"
	"time"
)

type OTPVerifier interface {
	RequestOTP(context.Context, string, string) error
	VerifyOTP(context.Context, string, string, string) error
}

type DispatchStatus string

const (
	StatusAssigned   DispatchStatus = "assigned"
	StatusDispatched DispatchStatus = "dispatched"
)

type WorkOrder struct {
	ID           string         `json:"id"`
	TechnicianID string         `json:"technician_id"`
	Phone        string         `json:"phone"`
	Status       DispatchStatus `json:"status"`
	Photos       []Photo        `json:"photos"`
	FollowUp     *FollowUp      `json:"follow_up,omitempty"`
}

type Photo struct {
	ObjectKey  string    `json:"object_key"`
	CapturedAt time.Time `json:"captured_at"`
}

type FollowUp struct {
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	mu     sync.RWMutex
	orders map[string]WorkOrder
}

func NewStore(seed ...WorkOrder) *Store {
	s := &Store{orders: make(map[string]WorkOrder, len(seed))}
	for _, order := range seed {
		s.orders[order.ID] = order
	}
	return s
}

func (s *Store) Get(id string) (WorkOrder, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	order, ok := s.orders[id]
	return order, ok
}

func (s *Store) DispatchAfterVerification(ctx context.Context, verifier OTPVerifier, orderID, phone, code, requestID string) (WorkOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return WorkOrder{}, ErrOrderNotFound
	}
	if order.Phone != phone {
		return WorkOrder{}, ErrPhoneMismatch
	}
	if err := verifier.VerifyOTP(ctx, phone, code, requestID); err != nil {
		return WorkOrder{}, err
	}
	order.Status = StatusDispatched
	s.orders[orderID] = order
	return order, nil
}

func (s *Store) AddPhoto(orderID string, photo Photo) (WorkOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return WorkOrder{}, ErrOrderNotFound
	}
	order.Photos = append(order.Photos, photo)
	s.orders[orderID] = order
	return order, nil
}

func (s *Store) SetFollowUp(orderID string, followUp FollowUp) (WorkOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return WorkOrder{}, ErrOrderNotFound
	}
	order.FollowUp = &followUp
	s.orders[orderID] = order
	return order, nil
}

var (
	ErrOrderNotFound = errors.New("work order not found")
	ErrPhoneMismatch = errors.New("phone does not match assigned technician")
)
