package server

import (
	"context"
	"testing"

	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/identity/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRegisterIdentityGrantsClusterAdminToPlatformIdentity(t *testing.T) {
	auth := &fakeAuthClient{}
	server := New(&fakeStore{}, auth)
	identityID := uuid.New()

	if _, err := server.RegisterIdentity(context.Background(), &identityv1.RegisterIdentityRequest{
		IdentityId:   identityID.String(),
		IdentityType: identityv1.IdentityType_IDENTITY_TYPE_PLATFORM,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(auth.writes) != 1 {
		t.Fatalf("expected one tuple write, got %d", len(auth.writes))
	}
	tuple := auth.writes[0]
	if tuple.GetUser() != identityObjectPrefix+identityID.String() {
		t.Fatalf("unexpected user: %s", tuple.GetUser())
	}
	if tuple.GetRelation() != adminRelation || tuple.GetObject() != clusterObject {
		t.Fatalf("unexpected tuple: %s on %s", tuple.GetRelation(), tuple.GetObject())
	}
}

// The Gateway re-registers the platform identity on every start, and the record
// already exists from the first one. The grant still has to converge, so a lost
// tuple is repaired without an operator.
func TestRegisterIdentityGrantsClusterAdminWhenAlreadyRegistered(t *testing.T) {
	auth := &fakeAuthClient{}
	identityID := uuid.New()
	fake := &fakeStore{
		registerErr:   store.AlreadyExists("identity"),
		identityTypes: map[uuid.UUID]int16{identityID: dbIdentityTypePlatform},
	}
	server := New(fake, auth)

	_, err := server.RegisterIdentity(context.Background(), &identityv1.RegisterIdentityRequest{
		IdentityId:   identityID.String(),
		IdentityType: identityv1.IdentityType_IDENTITY_TYPE_PLATFORM,
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists, got %v", err)
	}
	if len(auth.writes) != 1 {
		t.Fatalf("expected the grant to be written anyway, got %d writes", len(auth.writes))
	}
}

// An install that predates the platform type registered this identity as a
// user. Nothing else would ever correct the row.
func TestRegisterIdentityAdoptsAPreviouslyUserTypedPlatformIdentity(t *testing.T) {
	identityID := uuid.New()
	fake := &fakeStore{
		registerErr:   store.AlreadyExists("identity"),
		identityTypes: map[uuid.UUID]int16{identityID: dbIdentityTypeUser},
	}

	_, err := New(fake, &fakeAuthClient{}).RegisterIdentity(context.Background(), &identityv1.RegisterIdentityRequest{
		IdentityId:   identityID.String(),
		IdentityType: identityv1.IdentityType_IDENTITY_TYPE_PLATFORM,
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists, got %v", err)
	}
	if got := fake.identityTypes[identityID]; got != dbIdentityTypePlatform {
		t.Fatalf("expected the record to be adopted as platform, got type %d", got)
	}
}

// Anything else under that ID belongs to someone: a runner adopted as the
// platform admin would be handed cluster admin.
func TestRegisterIdentityRefusesToAdoptANonUserIdentity(t *testing.T) {
	identityID := uuid.New()
	auth := &fakeAuthClient{}
	fake := &fakeStore{
		registerErr:   store.AlreadyExists("identity"),
		identityTypes: map[uuid.UUID]int16{identityID: dbIdentityTypeRunner},
	}

	_, err := New(fake, auth).RegisterIdentity(context.Background(), &identityv1.RegisterIdentityRequest{
		IdentityId:   identityID.String(),
		IdentityType: identityv1.IdentityType_IDENTITY_TYPE_PLATFORM,
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %v", err)
	}
	if fake.identityTypes[identityID] != dbIdentityTypeRunner {
		t.Fatalf("expected the record to be left alone")
	}
	if len(auth.writes) != 0 {
		t.Fatalf("expected no grant, got %d writes", len(auth.writes))
	}
}

func TestRegisterIdentityGrantsNothingToOtherTypes(t *testing.T) {
	for _, identityType := range []identityv1.IdentityType{
		identityv1.IdentityType_IDENTITY_TYPE_USER,
		identityv1.IdentityType_IDENTITY_TYPE_RUNNER,
		identityv1.IdentityType_IDENTITY_TYPE_APP,
		identityv1.IdentityType_IDENTITY_TYPE_AGENT,
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
