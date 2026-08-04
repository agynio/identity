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
	nickname, err := normalizeNicknameStem(req.GetNickname())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "nickname: %v", err)
	}
	instanceSuffix, err := parseOptionalHandleSegment(req.InstanceSuffix)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "instance_suffix: %v", err)
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
	if err := validateInstanceSuffix(protoType, instanceSuffix); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "instance_suffix: %v", err)
	}

	if err := s.store.SetNickname(ctx, organizationID, identityID, installationID, nickname, instanceSuffix); err != nil {
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
	nickname, instanceSuffix, err := parseNicknameHandle(req.GetNickname(), req.InstanceSuffix)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "nickname: %v", err)
	}

	if err := s.authorizeNicknameRead(ctx, callerID, organizationID); err != nil {
		return nil, err
	}

	resolution, err := s.store.ResolveNickname(ctx, organizationID, nickname, instanceSuffix)
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
	if resolution.InstanceSuffix != nil {
		response.InstanceSuffix = resolution.InstanceSuffix
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

	// Membership, not can_view_threads: that resolves to owner-or-cluster-admin,
	// so no ordinary member of the organization could read its handles -- nor
	// could an agent instance, which is what leaves a raw identity id in front
	// of the model and "unknown participant" in the chat UI.
	allowed, err := s.checkPermission(ctx, callerID, "member", organizationID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "not a member of the organization")
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
		entries = append(entries, &identityv1.NicknameEntry{
			IdentityId:      id.String(),
			Nickname:        nickname.Nickname,
			InstanceSuffix: nickname.InstanceSuffix,
		})
	}

	return &identityv1.BatchGetNicknamesResponse{Entries: entries}, nil
}

func normalizeNicknameStem(value string) (string, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "@")
	if strings.Contains(trimmed, "#") {
		return "", fmt.Errorf("must not include instance suffix")
	}
	return normalizeHandleSegment(trimmed)
}

func normalizeHandleSegment(value string) (string, error) {
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

func parseNicknameHandle(value string, explicitSuffix *string) (string, *string, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "@")
	if trimmed == "" {
		return "", nil, fmt.Errorf("must be provided")
	}
	if explicitSuffix != nil && strings.Contains(trimmed, "#") {
		return "", nil, fmt.Errorf("must not include # when instance_suffix is set")
	}
	if explicitSuffix != nil {
		nickname, err := normalizeNicknameStem(trimmed)
		if err != nil {
			return "", nil, err
		}
		instanceSuffix, err := parseOptionalHandleSegment(explicitSuffix)
		if err != nil {
			return "", nil, fmt.Errorf("instance_suffix: %w", err)
		}
		return nickname, instanceSuffix, nil
	}
	parts := strings.Split(trimmed, "#")
	if len(parts) > 2 {
		return "", nil, fmt.Errorf("must contain at most one #")
	}
	nickname, err := normalizeHandleSegment(parts[0])
	if err != nil {
		return "", nil, err
	}
	if len(parts) == 1 {
		return nickname, nil, nil
	}
	instanceSuffix, err := normalizeHandleSegment(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("instance_suffix: %w", err)
	}
	return nickname, &instanceSuffix, nil
}

func parseOptionalHandleSegment(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized, err := normalizeHandleSegment(*value)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
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

func validateInstanceSuffix(identityType identityv1.IdentityType, instanceSuffix *string) error {
	if identityType == identityv1.IdentityType_IDENTITY_TYPE_AGENT_INSTANCE {
		if instanceSuffix == nil {
			return fmt.Errorf("required for agent_instance identities")
		}
		return nil
	}
	if instanceSuffix != nil {
		return fmt.Errorf("only valid for agent_instance identities")
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
