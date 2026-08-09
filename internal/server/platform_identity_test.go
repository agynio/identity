package server

import (
	"context"
	"testing"

	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/identity/internal/store"
	"github.com/google/uuid"
)

func TestEnsureRegistersAndGrants(t *testing.T) {
	auth := &fakeAuthClient{}
	identityID := uuid.New()

	if err := NewPlatformIdentity(&fakeStore{}, auth).Ensure(context.Background(), identityID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(auth.writes) != 1 {
		t.Fatalf("expected one tuple write, got %d", len(auth.writes))
	}
	tuple := auth.writes[0]
	if tuple.GetUser() != identityObjectPrefix+identityID.String() ||
		tuple.GetRelation() != adminRelation || tuple.GetObject() != clusterObject {
		t.Fatalf("unexpected tuple: %s %s %s", tuple.GetUser(), tuple.GetRelation(), tuple.GetObject())
	}
}

// Converging is the point: the grant is rewritten on every start, so a tuple
// lost out from under the platform is repaired without an operator.
func TestEnsureIsRepeatable(t *testing.T) {
	auth := &fakeAuthClient{}
	identityID := uuid.New()
	fake := &fakeStore{
		registerErr:   store.AlreadyExists("identity"),
		identityTypes: map[uuid.UUID]int16{identityID: dbIdentityTypePlatform},
	}

	if err := NewPlatformIdentity(fake, auth).Ensure(context.Background(), identityID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auth.writes) != 1 {
		t.Fatalf("expected the grant to be written anyway, got %d", len(auth.writes))
	}
}

// An install that predates the platform type carries this identity as a user.
func TestEnsureAdoptsAPreviouslyUserTypedRecord(t *testing.T) {
	identityID := uuid.New()
	fake := &fakeStore{
		registerErr:   store.AlreadyExists("identity"),
		identityTypes: map[uuid.UUID]int16{identityID: dbIdentityTypeUser},
	}

	if err := NewPlatformIdentity(fake, &fakeAuthClient{}).Ensure(context.Background(), identityID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := fake.identityTypes[identityID]; got != dbIdentityTypePlatform {
		t.Fatalf("expected the record adopted as platform, got type %d", got)
	}
}

// Anything else under that id belongs to someone: adopting a runner would hand
// it cluster admin.
func TestEnsureRefusesToAdoptANonUserRecord(t *testing.T) {
	identityID := uuid.New()
	auth := &fakeAuthClient{}
	fake := &fakeStore{
		registerErr:   store.AlreadyExists("identity"),
		identityTypes: map[uuid.UUID]int16{identityID: dbIdentityTypeRunner},
	}

	if err := NewPlatformIdentity(fake, auth).Ensure(context.Background(), identityID); err == nil {
		t.Fatal("expected an error")
	}
	if fake.identityTypes[identityID] != dbIdentityTypeRunner {
		t.Fatal("expected the record left alone")
	}
	if len(auth.writes) != 0 {
		t.Fatalf("expected no grant, got %d writes", len(auth.writes))
	}
}

// The grant is not reachable over the wire. Every service calls
// RegisterIdentity, so granting there would let any of them name the platform
// type and hand itself cluster admin.
func TestRegisterIdentityGrantsNothing(t *testing.T) {
	for _, identityType := range []identityv1.IdentityType{
		identityv1.IdentityType_IDENTITY_TYPE_PLATFORM,
		identityv1.IdentityType_IDENTITY_TYPE_USER,
		identityv1.IdentityType_IDENTITY_TYPE_RUNNER,
	} {
		auth := &fakeAuthClient{}
		server := New(&fakeStore{}, auth)
		if _, err := server.RegisterIdentity(context.Background(), &identityv1.RegisterIdentityRequest{
			IdentityId:   uuid.New().String(),
			IdentityType: identityType,
		}); err != nil {
			t.Fatalf("%s: unexpected error: %v", identityType, err)
		}
		if len(auth.writes) != 0 {
			t.Fatalf("%s: expected no tuple write, got %d", identityType, len(auth.writes))
		}
	}
}
