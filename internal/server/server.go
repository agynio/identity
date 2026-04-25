package server

import (
	"context"
	"errors"

	authorizationv1 "github.com/agynio/identity/.gen/go/agynio/api/authorization/v1"
	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/identity/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type identityStore interface {
	RegisterIdentity(context.Context, uuid.UUID, int16) error
	GetIdentityType(context.Context, uuid.UUID) (int16, error)
	BatchGetIdentityTypes(context.Context, []uuid.UUID) (map[uuid.UUID]int16, error)
	SetNickname(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string) error
	RemoveNickname(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error
	ResolveNickname(context.Context, uuid.UUID, string) (store.NicknameResolution, error)
	BatchGetNicknames(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]string, error)
}

type authorizationChecker interface {
	Check(context.Context, *authorizationv1.CheckRequest, ...grpc.CallOption) (*authorizationv1.CheckResponse, error)
}

type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	store               identityStore
	authorizationClient authorizationChecker
}

func New(store identityStore, authorizationClient authorizationChecker) *Server {
	return &Server{store: store, authorizationClient: authorizationClient}
}

func (s *Server) RegisterIdentity(ctx context.Context, req *identityv1.RegisterIdentityRequest) (*identityv1.RegisterIdentityResponse, error) {
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	identityType, err := identityTypeFromProto(req.GetIdentityType())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_type: %v", err)
	}
	if err := s.store.RegisterIdentity(ctx, identityID, identityType); err != nil {
		return nil, toStatusError(err)
	}
	return &identityv1.RegisterIdentityResponse{}, nil
}

func (s *Server) GetIdentityType(ctx context.Context, req *identityv1.GetIdentityTypeRequest) (*identityv1.GetIdentityTypeResponse, error) {
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	identityType, err := s.store.GetIdentityType(ctx, identityID)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoType, err := identityTypeToProto(identityType)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "internal error: %v", err)
	}
	return &identityv1.GetIdentityTypeResponse{IdentityType: protoType}, nil
}

func (s *Server) BatchGetIdentityTypes(ctx context.Context, req *identityv1.BatchGetIdentityTypesRequest) (*identityv1.BatchGetIdentityTypesResponse, error) {
	identityIDs := req.GetIdentityIds()
	if len(identityIDs) == 0 {
		return &identityv1.BatchGetIdentityTypesResponse{Entries: nil}, nil
	}

	ids := make([]uuid.UUID, 0, len(identityIDs))
	for i, identityID := range identityIDs {
		id, err := parseUUID(identityID)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "identity_ids[%d]: %v", i, err)
		}
		ids = append(ids, id)
	}

	identityTypes, err := s.store.BatchGetIdentityTypes(ctx, ids)
	if err != nil {
		return nil, toStatusError(err)
	}

	entries := make([]*identityv1.IdentityTypeEntry, 0, len(identityTypes))
	for _, id := range ids {
		identityType, ok := identityTypes[id]
		if !ok {
			continue
		}
		protoType, err := identityTypeToProto(identityType)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "internal error: %v", err)
		}
		entries = append(entries, &identityv1.IdentityTypeEntry{IdentityId: id.String(), IdentityType: protoType})
	}

	return &identityv1.BatchGetIdentityTypesResponse{Entries: entries}, nil
}

func parseUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, err
	}
	return id, nil
}

func toStatusError(err error) error {
	var notFound *store.NotFoundError
	if errors.As(err, &notFound) {
		return status.Error(codes.NotFound, notFound.Error())
	}
	var exists *store.AlreadyExistsError
	if errors.As(err, &exists) {
		return status.Error(codes.AlreadyExists, exists.Error())
	}
	return status.Errorf(codes.Internal, "internal error: %v", err)
}
