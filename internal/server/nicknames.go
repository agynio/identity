package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const nicknameMaxLength = 32

var nicknamePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

func (s *Server) SetNickname(ctx context.Context, req *identityv1.SetNicknameRequest) (*identityv1.SetNicknameResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	nickname, err := normalizeNickname(req.GetNickname())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "nickname: %v", err)
	}
	installationID, err := parseOptionalUUID(req.InstallationId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "installation_id: %v", err)
	}

	if err := s.authorizeNicknameWrite(ctx, callerID, organizationID, identityID); err != nil {
		return nil, err
	}

	identityType, err := s.store.GetIdentityType(ctx, identityID)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoType, err := identityTypeToProto(identityType)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "internal error: %v", err)
	}
	if err := validateInstallationID(protoType, installationID); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "installation_id: %v", err)
	}

	if err := s.store.SetNickname(ctx, organizationID, identityID, installationID, nickname); err != nil {
		return nil, toStatusError(err)
	}

	return &identityv1.SetNicknameResponse{}, nil
}

func (s *Server) RemoveNickname(ctx context.Context, req *identityv1.RemoveNicknameRequest) (*identityv1.RemoveNicknameResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	installationID, err := parseOptionalUUID(req.InstallationId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "installation_id: %v", err)
	}

	if err := s.authorizeNicknameWrite(ctx, callerID, organizationID, identityID); err != nil {
		return nil, err
	}

	identityType, err := s.store.GetIdentityType(ctx, identityID)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoType, err := identityTypeToProto(identityType)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "internal error: %v", err)
	}
	if err := validateInstallationID(protoType, installationID); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "installation_id: %v", err)
	}

	if err := s.store.RemoveNickname(ctx, organizationID, identityID, installationID); err != nil {
		return nil, toStatusError(err)
	}

	return &identityv1.RemoveNicknameResponse{}, nil
}

func (s *Server) ResolveNickname(ctx context.Context, req *identityv1.ResolveNicknameRequest) (*identityv1.ResolveNicknameResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	nickname, err := normalizeNickname(req.GetNickname())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "nickname: %v", err)
	}

	if err := s.authorizeNicknameRead(ctx, callerID, organizationID); err != nil {
		return nil, err
	}

	resolution, err := s.store.ResolveNickname(ctx, organizationID, nickname)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoType, err := identityTypeToProto(resolution.IdentityType)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "internal error: %v", err)
	}

	response := &identityv1.ResolveNicknameResponse{
		IdentityId:   resolution.IdentityID.String(),
		IdentityType: protoType,
	}
	if resolution.InstallationID != nil {
		installationID := resolution.InstallationID.String()
		response.InstallationId = &installationID
	}
	return response, nil
}

func (s *Server) BatchGetNicknames(ctx context.Context, req *identityv1.BatchGetNicknamesRequest) (*identityv1.BatchGetNicknamesResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}

	allowed, err := s.checkPermission(ctx, callerID, "can_view_threads", organizationID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "missing permission to view thread nicknames")
	}

	identityIDs := req.GetIdentityIds()
	if len(identityIDs) == 0 {
		return &identityv1.BatchGetNicknamesResponse{Entries: nil}, nil
	}

	ids := make([]uuid.UUID, 0, len(identityIDs))
	for i, identityID := range identityIDs {
		id, err := parseUUID(identityID)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "identity_ids[%d]: %v", i, err)
		}
		ids = append(ids, id)
	}

	nicknames, err := s.store.BatchGetNicknames(ctx, organizationID, ids)
	if err != nil {
		return nil, toStatusError(err)
	}

	entries := make([]*identityv1.NicknameEntry, 0, len(nicknames))
	for _, id := range ids {
		nickname, ok := nicknames[id]
		if !ok {
			continue
		}
		entries = append(entries, &identityv1.NicknameEntry{IdentityId: id.String(), Nickname: nickname})
	}

	return &identityv1.BatchGetNicknamesResponse{Entries: entries}, nil
}

func normalizeNickname(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("must be provided")
	}
	if len(trimmed) > nicknameMaxLength {
		return "", fmt.Errorf("must be %d characters or fewer", nicknameMaxLength)
	}
	if !nicknamePattern.MatchString(trimmed) {
		return "", fmt.Errorf("must match %s", nicknamePattern.String())
	}
	return trimmed, nil
}

func parseOptionalUUID(value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, fmt.Errorf("must be provided")
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func validateInstallationID(identityType identityv1.IdentityType, installationID *uuid.UUID) error {
	if identityType == identityv1.IdentityType_IDENTITY_TYPE_APP {
		if installationID == nil {
			return fmt.Errorf("required for app identities")
		}
		return nil
	}
	if installationID != nil {
		return fmt.Errorf("only valid for app identities")
	}
	return nil
}

func (s *Server) authorizeNicknameWrite(ctx context.Context, callerID uuid.UUID, organizationID uuid.UUID, identityID uuid.UUID) error {
	if callerID == identityID {
		allowed, err := s.checkPermission(ctx, callerID, "member", organizationID)
		if err != nil {
			return status.Errorf(codes.Internal, "authorization check: %v", err)
		}
		if !allowed {
			return status.Error(codes.PermissionDenied, "missing permission to manage nickname")
		}
		return nil
	}

	allowed, err := s.checkPermission(ctx, callerID, "can_manage_members", organizationID)
	if err != nil {
		return status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	if allowed {
		return nil
	}
	allowed, err = s.checkPermission(ctx, callerID, "can_add_member", organizationID)
	if err != nil {
		return status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	if !allowed {
		return status.Error(codes.PermissionDenied, "missing permission to manage nickname")
	}
	return nil
}

func (s *Server) authorizeNicknameRead(ctx context.Context, callerID uuid.UUID, organizationID uuid.UUID) error {
	allowed, err := s.checkPermission(ctx, callerID, "member", organizationID)
	if err != nil {
		return status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	if allowed {
		return nil
	}
	allowed, err = s.checkPermission(ctx, callerID, "can_view_threads", organizationID)
	if err != nil {
		return status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	if !allowed {
		return status.Error(codes.PermissionDenied, "missing permission to view nicknames")
	}
	return nil
}
