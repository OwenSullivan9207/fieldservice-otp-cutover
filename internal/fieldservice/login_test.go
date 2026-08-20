package fieldservice

import (
	"context"
	"errors"
	"testing"
)

type verifierStub struct{ err error }

func (v verifierStub) RequestOTP(context.Context, string, string) error { return v.err }
func (v verifierStub) VerifyOTP(context.Context, string, string, string) error {
	return v.err
}

func TestDispatchAfterVerification(t *testing.T) {
	rejected := errors.New("code rejected")
	tests := []struct {
		name       string
		phone      string
		verifyErr  error
		wantStatus DispatchStatus
		wantErr    error
	}{
		{name: "verified technician is dispatched", phone: "+14155550100", wantStatus: StatusDispatched},
		{name: "rejected code keeps assignment", phone: "+14155550100", verifyErr: rejected, wantStatus: StatusAssigned, wantErr: rejected},
		{name: "different phone keeps assignment", phone: "+14155550199", wantStatus: StatusAssigned, wantErr: ErrPhoneMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore(WorkOrder{ID: "WO-1042", TechnicianID: "tech-7", Phone: "+14155550100", Status: StatusAssigned})
			_, err := store.DispatchAfterVerification(context.Background(), verifierStub{err: tt.verifyErr}, "WO-1042", tt.phone, "482901", "verify-test")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			got, _ := store.Get("WO-1042")
			if got.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, tt.wantStatus)
			}
		})
	}
}
