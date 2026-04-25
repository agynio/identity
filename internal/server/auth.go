package server

import (
	"context"
	"fmt"
	"strings"

	authorizationv1 "github.com/agynio/identity/.gen/go/agynio/api/authorization/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

const (
	identityHeaderKey        = "x-identity-id"
	identityObjectPrefix     = "identity:"
	organizationObjectPrefix = "organization:"
)

func identityIDFromContext(ctx context.Context) (uuid.UUID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return uuid.Nil, fmt.Errorf("metadata missing")
	}
	values := md.Get(identityHeaderKey)
	if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
		return uuid.Nil, fmt.Errorf("%s not found in metadata", identityHeaderKey)
	}
	return uuid.Parse(strings.TrimSpace(values[0]))
}

func (s *Server) checkPermission(ctx context.Context, identityID uuid.UUID, relation string, organizationID uuid.UUID) (bool, error) {
	if s.authorizationClient == nil {
		return false, fmt.Errorf("authorization client not configured")
	}
	response, err := s.authorizationClient.Check(ctx, &authorizationv1.CheckRequest{
		TupleKey: &authorizationv1.TupleKey{
			User:     fmt.Sprintf("%s%s", identityObjectPrefix, identityID.String()),
			Relation: relation,
			Object:   fmt.Sprintf("%s%s", organizationObjectPrefix, organizationID.String()),
		},
	})
	if err != nil {
		return false, err
	}
	return response.GetAllowed(), nil
}
