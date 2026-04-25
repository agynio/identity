package server

import (
	"context"
	"fmt"
	"testing"

	authorizationv1 "github.com/agynio/identity/.gen/go/agynio/api/authorization/v1"
	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/identity/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeAuthClient struct {
	allowed     bool
	lastRequest *authorizationv1.CheckRequest
	err         error
}

func (f *fakeAuthClient) Check(_ context.Context, req *authorizationv1.CheckRequest, _ ...grpc.CallOption) (*authorizationv1.CheckResponse, error) {
	f.lastRequest = req
	if f.err != nil {
		return nil, f.err
	}
	return &authorizationv1.CheckResponse{Allowed: f.allowed}, nil
}

type fakeStore struct {
	batchNicknames     map[uuid.UUID]string
	batchErr           error
	lastOrganizationID uuid.UUID
	lastIdentityIDs    []uuid.UUID
}

func (f *fakeStore) RegisterIdentity(context.Context, uuid.UUID, int16) error {
	return nil
}

func (f *fakeStore) GetIdentityType(context.Context, uuid.UUID) (int16, error) {
	return dbIdentityTypeUser, nil
}

func (f *fakeStore) BatchGetIdentityTypes(context.Context, []uuid.UUID) (map[uuid.UUID]int16, error) {
	return map[uuid.UUID]int16{}, nil
}

func (f *fakeStore) SetNickname(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string) error {
	return nil
}

func (f *fakeStore) RemoveNickname(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error {
	return nil
}

func (f *fakeStore) ResolveNickname(context.Context, uuid.UUID, string) (store.NicknameResolution, error) {
	return store.NicknameResolution{}, nil
}

func (f *fakeStore) BatchGetNicknames(_ context.Context, organizationID uuid.UUID, identityIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	f.lastOrganizationID = organizationID
	f.lastIdentityIDs = identityIDs
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	return f.batchNicknames, nil
}

func TestBatchGetNicknamesOmitsMissing(t *testing.T) {
	organizationID := uuid.New()
	callerID := uuid.New()
	firstIdentity := uuid.New()
	secondIdentity := uuid.New()

	store := &fakeStore{batchNicknames: map[uuid.UUID]string{secondIdentity: "runner"}}
	auth := &fakeAuthClient{allowed: true}
	server := New(store, auth)

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(identityHeaderKey, callerID.String()))
	response, err := server.BatchGetNicknames(ctx, &identityv1.BatchGetNicknamesRequest{
		OrganizationId: organizationID.String(),
		IdentityIds:    []string{firstIdentity.String(), secondIdentity.String()},
	})
	require.NoError(t, err)
	require.Len(t, response.Entries, 1)
	require.Equal(t, secondIdentity.String(), response.Entries[0].GetIdentityId())
	require.Equal(t, "runner", response.Entries[0].GetNickname())

	require.NotNil(t, auth.lastRequest)
	require.Equal(t, "can_view_threads", auth.lastRequest.GetTupleKey().GetRelation())
	require.Equal(t, fmt.Sprintf("%s%s", identityObjectPrefix, callerID.String()), auth.lastRequest.GetTupleKey().GetUser())
	require.Equal(t, fmt.Sprintf("%s%s", organizationObjectPrefix, organizationID.String()), auth.lastRequest.GetTupleKey().GetObject())
}

func TestBatchGetNicknamesMissingIdentityHeader(t *testing.T) {
	server := New(&fakeStore{}, &fakeAuthClient{allowed: true})
	_, err := server.BatchGetNicknames(context.Background(), &identityv1.BatchGetNicknamesRequest{OrganizationId: uuid.NewString()})
	requireStatusCode(t, err, codes.Unauthenticated)
}

func TestBatchGetNicknamesPermissionDenied(t *testing.T) {
	callerID := uuid.New()
	server := New(&fakeStore{}, &fakeAuthClient{allowed: false})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(identityHeaderKey, callerID.String()))
	_, err := server.BatchGetNicknames(ctx, &identityv1.BatchGetNicknamesRequest{
		OrganizationId: uuid.NewString(),
		IdentityIds:    []string{uuid.NewString()},
	})
	requireStatusCode(t, err, codes.PermissionDenied)
}

func TestBatchGetNicknamesInvalidIdentityID(t *testing.T) {
	callerID := uuid.New()
	server := New(&fakeStore{}, &fakeAuthClient{allowed: true})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(identityHeaderKey, callerID.String()))
	_, err := server.BatchGetNicknames(ctx, &identityv1.BatchGetNicknamesRequest{
		OrganizationId: uuid.NewString(),
		IdentityIds:    []string{"bad"},
	})
	requireStatusCode(t, err, codes.InvalidArgument)
}

func requireStatusCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	statusErr, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, code, statusErr.Code())
}
