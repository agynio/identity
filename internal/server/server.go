package server

import (
	"context"
	"errors"
	"fmt"

	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/identity/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	store *store.Store
}

func New(store *store.Store) *Server {
	return &Server{store: store}
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
	if value == "" {
		return uuid.UUID{}, fmt.Errorf("value is empty")
	}
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
