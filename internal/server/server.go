package server

import (
	"context"
	"errors"
	"fmt"

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
	SetIdentityType(context.Context, uuid.UUID, int16) error
	GetIdentityType(context.Context, uuid.UUID) (int16, error)
	BatchGetIdentityTypes(context.Context, []uuid.UUID) (map[uuid.UUID]int16, error)
	SetNickname(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string, *string) error
	RemoveNickname(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error
	ResolveNickname(context.Context, uuid.UUID, string, *string) (store.NicknameResolution, error)
	BatchGetNicknames(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]store.NicknameEntry, error)
}

type authorizationChecker interface {
	Check(context.Context, *authorizationv1.CheckRequest, ...grpc.CallOption) (*authorizationv1.CheckResponse, error)
	Write(context.Context, *authorizationv1.WriteRequest, ...grpc.CallOption) (*authorizationv1.WriteResponse, error)
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
	registerErr := s.store.RegisterIdentity(ctx, identityID, identityType)
	// A platform identity is a cluster admin by definition, so the grant is
	// ensured here rather than by whoever configured the identity. It is
	// re-registered on every Gateway start, and the grant converges on each
	// one, so a tuple lost out from under it is repaired without an operator.
	if identityType == dbIdentityTypePlatform && (registerErr == nil || isAlreadyExists(registerErr)) {
		// Installs that predate the platform type carry this identity as a
		// user, and nothing else would ever correct the row.
		if isAlreadyExists(registerErr) {
			if err := s.adoptPlatformIdentity(ctx, identityID); err != nil {
				return nil, status.Errorf(codes.Internal, "adopt platform identity: %v", err)
			}
		}
		if err := s.grantClusterAdmin(ctx, identityID); err != nil {
			return nil, status.Errorf(codes.Internal, "grant cluster admin: %v", err)
		}
	}
	if registerErr != nil {
		return nil, toStatusError(registerErr)
	}
	return &identityv1.RegisterIdentityResponse{}, nil
}

// adoptPlatformIdentity rewrites an existing record to the platform type. Only
// widens from the user type the Gateway used to register it with: anything else
// under this ID is someone else's identity and is left alone.
func (s *Server) adoptPlatformIdentity(ctx context.Context, identityID uuid.UUID) error {
	current, err := s.store.GetIdentityType(ctx, identityID)
	if err != nil {
		return err
	}
	if current == dbIdentityTypePlatform {
		return nil
	}
	if current != dbIdentityTypeUser {
		return fmt.Errorf("identity %s is registered as type %d, not a platform identity", identityID, current)
	}
	return s.store.SetIdentityType(ctx, identityID, dbIdentityTypePlatform)
}

// grantClusterAdmin writes the platform admin identity's admin relation on
// cluster:global through the service that owns it. Writing an existing tuple is
// not an error, so this is safe to repeat.
func (s *Server) grantClusterAdmin(ctx context.Context, identityID uuid.UUID) error {
	if s.authorizationClient == nil {
		return errors.New("authorization client not configured")
	}
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
		Writes: []*authorizationv1.TupleKey{{
			User:     identityObjectPrefix + identityID.String(),
			Relation: adminRelation,
			Object:   clusterObject,
		}},
	})
	if err != nil && status.Code(err) == codes.AlreadyExists {
		return nil
	}
	return err
}

func isAlreadyExists(err error) bool {
	var exists *store.AlreadyExistsError
	return errors.As(err, &exists)
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
