package server

import (
	"context"
	"errors"
	"fmt"
	"regexp"

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

func (s *Server) SetNickname(ctx context.Context, req *identityv1.SetNicknameRequest) (*identityv1.SetNicknameResponse, error) {
	orgID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	nickname, err := parseNickname(req.GetNickname())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "nickname: %v", err)
	}

	var installationID *uuid.UUID
	if req.InstallationId != nil {
		parsed, err := parseUUID(req.GetInstallationId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "installation_id: %v", err)
		}
		installationID = &parsed
	}

	if err := s.store.SetNickname(ctx, orgID, identityID, nickname, installationID); err != nil {
		return nil, toStatusError(err)
	}
	return &identityv1.SetNicknameResponse{}, nil
}

func (s *Server) RemoveNickname(ctx context.Context, req *identityv1.RemoveNicknameRequest) (*identityv1.RemoveNicknameResponse, error) {
	orgID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}

	var installationID *uuid.UUID
	if req.InstallationId != nil {
		parsed, err := parseUUID(req.GetInstallationId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "installation_id: %v", err)
		}
		installationID = &parsed
	}

	if err := s.store.RemoveNickname(ctx, orgID, identityID, installationID); err != nil {
		return nil, toStatusError(err)
	}
	return &identityv1.RemoveNicknameResponse{}, nil
}

func (s *Server) ResolveNickname(ctx context.Context, req *identityv1.ResolveNicknameRequest) (*identityv1.ResolveNicknameResponse, error) {
	orgID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	nickname, err := parseNickname(req.GetNickname())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "nickname: %v", err)
	}

	resolved, err := s.store.ResolveNickname(ctx, orgID, nickname)
	if err != nil {
		return nil, toStatusError(err)
	}
	identityType, err := identityTypeToProto(resolved.IdentityType)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "internal error: %v", err)
	}

	resp := &identityv1.ResolveNicknameResponse{
		IdentityId:   resolved.IdentityID.String(),
		IdentityType: identityType,
	}
	if resolved.InstallationID != nil {
		installationID := resolved.InstallationID.String()
		resp.InstallationId = &installationID
	}
	return resp, nil
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

var nicknamePattern = regexp.MustCompile("^[a-z0-9_-]+$")

func parseNickname(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("value is empty")
	}
	if len(value) > 32 {
		return "", fmt.Errorf("value exceeds 32 characters")
	}
	if !nicknamePattern.MatchString(value) {
		return "", fmt.Errorf("value must match ^[a-z0-9_-]+$")
	}
	return value, nil
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
