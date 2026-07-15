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
	batchNicknames     map[uuid.UUID]store.NicknameEntry
	batchErr           error
	lastOrganizationID uuid.UUID
	lastIdentityIDs    []uuid.UUID
	identityTypes      map[uuid.UUID]int16
	lastNickname       string
	lastInstanceSuffix *string
	resolution         store.NicknameResolution
}

func (f *fakeStore) RegisterIdentity(context.Context, uuid.UUID, int16) error {
	return nil
}

func (f *fakeStore) GetIdentityType(_ context.Context, identityID uuid.UUID) (int16, error) {
	if f.identityTypes != nil {
		if identityType, ok := f.identityTypes[identityID]; ok {
			return identityType, nil
		}
	}
	return dbIdentityTypeUser, nil
}

func (f *fakeStore) BatchGetIdentityTypes(context.Context, []uuid.UUID) (map[uuid.UUID]int16, error) {
	return map[uuid.UUID]int16{}, nil
}

func (f *fakeStore) SetNickname(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *uuid.UUID, nickname string, instanceSuffix *string) error {
	f.lastNickname = nickname
	f.lastInstanceSuffix = instanceSuffix
	return nil
}

func (f *fakeStore) RemoveNickname(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error {
	return nil
}

func (f *fakeStore) ResolveNickname(_ context.Context, _ uuid.UUID, nickname string, instanceSuffix *string) (store.NicknameResolution, error) {
	f.lastNickname = nickname
	f.lastInstanceSuffix = instanceSuffix
	return f.resolution, nil
}

func (f *fakeStore) BatchGetNicknames(_ context.Context, organizationID uuid.UUID, identityIDs []uuid.UUID) (map[uuid.UUID]store.NicknameEntry, error) {
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

	store := &fakeStore{batchNicknames: map[uuid.UUID]store.NicknameEntry{secondIdentity: {Nickname: "runner"}}}
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

func TestSetNicknameAcceptsAgentInstanceSuffix(t *testing.T) {
	organizationID := uuid.New()
	callerID := uuid.New()
	identityID := uuid.New()
	instanceSuffix := "planning-run-1"
	store := &fakeStore{identityTypes: map[uuid.UUID]int16{identityID: dbIdentityTypeAgentInstance}}
	server := New(store, &fakeAuthClient{allowed: true})

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(identityHeaderKey, callerID.String()))
	_, err := server.SetNickname(ctx, &identityv1.SetNicknameRequest{
		OrganizationId: organizationID.String(),
		IdentityId:     identityID.String(),
		Nickname:       "@bob",
		InstanceSuffix: &instanceSuffix,
	})
	require.NoError(t, err)
	require.Equal(t, "bob", store.lastNickname)
	require.NotNil(t, store.lastInstanceSuffix)
	require.Equal(t, instanceSuffix, *store.lastInstanceSuffix)
}

func TestSetNicknameRequiresSuffixForAgentInstance(t *testing.T) {
	organizationID := uuid.New()
	callerID := uuid.New()
	identityID := uuid.New()
	store := &fakeStore{identityTypes: map[uuid.UUID]int16{identityID: dbIdentityTypeAgentInstance}}
	server := New(store, &fakeAuthClient{allowed: true})

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(identityHeaderKey, callerID.String()))
	_, err := server.SetNickname(ctx, &identityv1.SetNicknameRequest{
		OrganizationId: organizationID.String(),
		IdentityId:     identityID.String(),
		Nickname:       "bob",
	})
	requireStatusCode(t, err, codes.InvalidArgument)
}

func TestSetNicknameRejectsSuffixForNonInstance(t *testing.T) {
	organizationID := uuid.New()
	callerID := uuid.New()
	identityID := uuid.New()
	instanceSuffix := "7a2f"
	store := &fakeStore{identityTypes: map[uuid.UUID]int16{identityID: dbIdentityTypeAgent}}
	server := New(store, &fakeAuthClient{allowed: true})

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(identityHeaderKey, callerID.String()))
	_, err := server.SetNickname(ctx, &identityv1.SetNicknameRequest{
		OrganizationId: organizationID.String(),
		IdentityId:     identityID.String(),
		Nickname:       "bob",
		InstanceSuffix: &instanceSuffix,
	})
	requireStatusCode(t, err, codes.InvalidArgument)
}

func TestResolveNicknameParsesInstanceHandle(t *testing.T) {
	organizationID := uuid.New()
	callerID := uuid.New()
	identityID := uuid.New()
	instanceSuffix := "7a2f"
	store := &fakeStore{resolution: store.NicknameResolution{
		IdentityID:     identityID,
		IdentityType:   dbIdentityTypeAgentInstance,
		InstanceSuffix: &instanceSuffix,
	}}
	server := New(store, &fakeAuthClient{allowed: true})

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(identityHeaderKey, callerID.String()))
	response, err := server.ResolveNickname(ctx, &identityv1.ResolveNicknameRequest{
		OrganizationId: organizationID.String(),
		Nickname:       "@bob#7a2f",
	})
	require.NoError(t, err)
	require.Equal(t, "bob", store.lastNickname)
	require.NotNil(t, store.lastInstanceSuffix)
	require.Equal(t, instanceSuffix, *store.lastInstanceSuffix)
	require.Equal(t, identityID.String(), response.GetIdentityId())
	require.Equal(t, identityv1.IdentityType_IDENTITY_TYPE_AGENT_INSTANCE, response.GetIdentityType())
	require.Equal(t, instanceSuffix, response.GetInstanceSuffix())
}

func TestResolveNicknameRejectsDuplicateSuffixInputs(t *testing.T) {
	organizationID := uuid.New()
	callerID := uuid.New()
	instanceSuffix := "7a2f"
	server := New(&fakeStore{}, &fakeAuthClient{allowed: true})

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(identityHeaderKey, callerID.String()))
	_, err := server.ResolveNickname(ctx, &identityv1.ResolveNicknameRequest{
		OrganizationId: organizationID.String(),
		Nickname:       "bob#7a2f",
		InstanceSuffix: &instanceSuffix,
	})
	requireStatusCode(t, err, codes.InvalidArgument)
}

func TestBatchGetNicknamesReturnsInstanceSuffix(t *testing.T) {
	organizationID := uuid.New()
	callerID := uuid.New()
	identityID := uuid.New()
	instanceSuffix := "7a2f"
	store := &fakeStore{batchNicknames: map[uuid.UUID]store.NicknameEntry{identityID: {Nickname: "bob", InstanceSuffix: &instanceSuffix}}}
	server := New(store, &fakeAuthClient{allowed: true})

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(identityHeaderKey, callerID.String()))
	response, err := server.BatchGetNicknames(ctx, &identityv1.BatchGetNicknamesRequest{
		OrganizationId: organizationID.String(),
		IdentityIds:    []string{identityID.String()},
	})
	require.NoError(t, err)
	require.Len(t, response.Entries, 1)
	require.Equal(t, "bob", response.Entries[0].GetNickname())
	require.Equal(t, instanceSuffix, response.Entries[0].GetInstanceSuffix())
}
